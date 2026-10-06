package cli

import (
	"strings"
	"testing"
	"time"

	icaro "github.com/mvanhorn/printing-press-library/library/other/ars-sicilia/internal/icaroclient"
)

func misuraAl(giorno string, now time.Time) *latenzaMisura {
	f, err := time.Parse("2006-01-02", giorno)
	if err != nil {
		panic(err)
	}
	oggi := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return &latenzaMisura{Frontiera: f, Giorni: int(oggi.Sub(f).Hours() / 24)}
}

// Un cerca vuoto con --data che va oltre la frontiera della fonte deve dire che
// può essere latenza; uno dentro la parte già pubblicata no, e con risultati mai.
func TestLatenzaHint(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	// Frontiera lontana: serve ai casi che provano la guardia sulla finestra,
	// dove la misura non deve essere ciò che decide.
	vecchia := misuraAl("2019-01-01", now)
	cases := []struct {
		name string
		recs []icaro.Record
		data string
		mis  *latenzaMisura
		want bool
	}{
		{"range recente vuoto", nil, "2026-08-01:2026-09-04", vecchia, true},
		{"data singola recente vuota", nil, "2026-09-02", vecchia, true},
		{"range oltre la guardia di spesa", nil, "2020-11-01:2020-12-31", vecchia, false},
		{"al limite dei 120 giorni", nil, "2026-05-07", vecchia, true},
		{"oltre i 120 giorni", nil, "2026-05-06", vecchia, false},
		{"senza --data", nil, "", vecchia, false},
		{"con risultati", []icaro.Record{{}}, "2026-09-02", vecchia, false},
		{"data non ISO", nil, "260902", vecchia, false},
		{"finestra tutta nel futuro", nil, "2027-01-01:2027-12-31", vecchia, false},
		{"data singola futura", nil, "2026-09-10", vecchia, false},
		{"a cavallo di oggi: copre giorni recenti", nil, "2026-08-15:2026-09-30", vecchia, true},
		{"inizio vecchio, fine futura: si valuta su oggi", nil, "2026-07-01:2026-12-31", vecchia, true},

		// Il cuore della correzione: la stessa finestra parla o tace a seconda
		// di dove arriva davvero la fonte, non di una banda scritta a mano.
		{"periodo oltre la frontiera: parla", nil, "2026-09-02", misuraAl("2026-08-25", now), true},
		{"periodo dentro il pubblicato: tace", nil, "2026-08-20", misuraAl("2026-08-25", now), false},
		{"periodo esattamente sulla frontiera: tace", nil, "2026-08-25", misuraAl("2026-08-25", now), false},
		{"archivio fresco, assenza vera: tace", nil, "2026-09-01", misuraAl("2026-09-03", now), false},
		{"archivio molto indietro: parla", nil, "2026-08-01", misuraAl("2026-07-22", now), true},
		{"misura mancante: parla lo stesso", nil, "2026-09-02", nil, true},
	}
	for _, c := range cases {
		got := latenzaHint(c.recs, "interrogazioni", map[string]string{"data": c.data}, now, c.mis)
		if (got != "") != c.want {
			t.Errorf("%s: got %q, want hint=%v", c.name, got, c.want)
		}
	}
}

// Quando la misura c'è, l'hint la riporta: è tutto il punto della modifica.
// Quando manca, non deve comparire una cifra inventata al suo posto.
func TestLatenzaHintRiportaLaMisura(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	params := map[string]string{"data": "2026-09-16"}

	con := latenzaHint(nil, "sommari", params, now, misuraAl("2026-09-08", now))
	if !strings.Contains(con, "2026-09-08") || !strings.Contains(con, "10 giorni") {
		t.Errorf("l'hint deve dire dove si ferma la fonte e di quanto: %q", con)
	}
	if strings.Contains(con, "9-45") {
		t.Errorf("la banda scritta a mano non deve sopravvivere: %q", con)
	}

	senza := latenzaHint(nil, "sommari", params, now, nil)
	if !strings.Contains(senza, "--resources sommari") {
		t.Errorf("senza misura l'hint deve dire dove misurarla: %q", senza)
	}
	if strings.Contains(senza, "giorni fa") {
		t.Errorf("senza misura non deve uscire una cifra: %q", senza)
	}
}

// L'hint non deve mai suggerire un probe a una riga sola: `sync_coverage.go`
// documenta che l'ordine della fonte non è uniforme, quindi `--limit 1` darebbe
// la data sbagliata proprio dove si decide «latenza o assenza».
func TestLatenzaHintNonSuggerisceLimit1(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	params := map[string]string{"data": "2026-09-16"}
	for _, h := range []string{
		latenzaHint(nil, "sommari", params, now, misuraAl("2026-09-08", now)),
		latenzaHint(nil, "sommari", params, now, nil),
	} {
		if strings.Contains(h, "limit 1") {
			t.Errorf("l'hint suggerisce un probe falso: %q", h)
		}
	}
}

// La sonda si porta dietro i filtri che scelgono una fetta dell'archivio, non
// quelli che scelgono un contenuto: misurare la frontiera dei documenti che
// contengono una parola e chiamarla frontiera dell'archivio sarebbe la stessa
// cifra falsa che questa modifica toglie.
func TestParamsSondaSoloFiltriStrutturali(t *testing.T) {
	arc := icaro.BySlug("sommari")
	if arc == nil {
		t.Fatal("archivio sommari non trovato")
	}
	got := paramsSonda(*arc, map[string]string{
		"data":        "2026-09-16",
		"commissione": "QUARTA",
		"legisl":      "18",
		"testo":       "bilancio",
		"frase":       "criteri oggettivi",
		"firmatario":  "Varrica",
	}, 2026)
	if got["commissione"] != "QUARTA" || got["legisl"] != "18" {
		t.Errorf("i filtri strutturali devono passare: %v", got)
	}
	for _, k := range []string{"testo", "frase", "firmatario"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s non deve entrare nella sonda: %v", k, got)
		}
	}
	if got["anno"] != "2026" {
		t.Errorf("la sonda deve restringersi all'anno in corso: %v", got)
	}
	if v, ok := got["data"]; ok && v == "2026-09-16" {
		t.Errorf("la finestra vuota non va rimessa nella sonda: %v", got)
	}
}

// `biblioteca` non ha né anno né colonna data: non è misurabile, e la sonda
// deve dirlo rinunciando invece di costruire una richiesta senza senso.
func TestParamsSondaArchivioSenzaTempo(t *testing.T) {
	arc := icaro.BySlug("biblioteca")
	if arc == nil {
		t.Fatal("archivio biblioteca non trovato")
	}
	if got := paramsSonda(*arc, map[string]string{"data": "2026-09-16"}, 2026); got != nil {
		t.Errorf("archivio senza dimensione temporale: la sonda doveva rinunciare, ha dato %v", got)
	}
}

// La frase deve nominare i filtri con cui la frontiera è stata misurata, e solo
// quelli. `testo` non entra nella sonda: prometterlo nella prosa consegnerebbe
// la data dell'archivio spacciata per quella dei documenti che contengono la
// parola, cioè la stessa cifra falsa che questa modifica toglie dal numero.
func TestLatenzaHintDichiaraSoloIFiltriMisurati(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	m := misuraAl("2026-09-08", now)

	conCommissione := latenzaHint(nil, "sommari", map[string]string{
		"data": "2026-09-16", "commissione": "QUARTA", "legisl": "18",
	}, now, m)
	if !strings.Contains(conCommissione, "la commissione QUARTA") || !strings.Contains(conCommissione, "la legislatura 18") {
		t.Errorf("i filtri misurati vanno nominati: %q", conCommissione)
	}

	soloTesto := latenzaHint(nil, "interrogazioni", map[string]string{
		"data": "2026-09-16", "testo": "bilancio",
	}, now, m)
	if strings.Contains(soloTesto, "bilancio") {
		t.Errorf("un filtro che la sonda non ha usato non va nominato: %q", soloTesto)
	}
	if strings.Contains(soloTesto, ", per ") || strings.Contains(soloTesto, "filtri di questa ricerca") {
		t.Errorf("senza filtri misurati la frase non deve prometterne: %q", soloTesto)
	}
}

// `pareri` e `biblioteca` non sono misurabili: lì l'avviso senza cifra
// rimanderebbe a `sync coverage`, che risponde «non misurabile». Mandare
// qualcuno a leggere un non-dato è rumore, quindi si tace. Sugli archivi
// misurabili invece la sonda fallita lascia comunque l'avviso.
func TestLatenzaHintTaceSugliArchiviNonMisurabili(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	params := map[string]string{"data": "2026-09-16"}
	for _, slug := range []string{"pareri", "biblioteca"} {
		if got := latenzaHint(nil, slug, params, now, nil); got != "" {
			t.Errorf("%s non è misurabile: l'hint doveva tacere, ha detto %q", slug, got)
		}
	}
	if got := latenzaHint(nil, "interrogazioni", params, now, nil); got == "" {
		t.Error("su un archivio misurabile la sonda fallita deve lasciare l'avviso senza cifra")
	}
}

// `leggi` è l'unico archivio misurabile senza `--data`: filtra per `--anno`, e
// ignorarlo lasciava scoperto proprio l'archivio col ritardo peggiore. Un anno è
// una finestra dal 1º gennaio al 31 dicembre, e quella in corso finisce nel
// futuro, quindi si valuta come se finisse oggi.
func TestLatenzaHintLeggeLAnnoComeFinestra(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	m := misuraAl("2026-08-04", now)

	inCorso := latenzaHint(nil, "leggi", map[string]string{"anno": "2026"}, now, m)
	if inCorso == "" {
		t.Error("l'anno in corso arriva a oggi: l'avviso deve uscire")
	}
	if !strings.Contains(inCorso, "2026-08-04") {
		t.Errorf("con la misura in mano la data va detta: %q", inCorso)
	}

	if got := latenzaHint(nil, "leggi", map[string]string{"anno": "2019"}, now, m); got != "" {
		t.Errorf("un anno chiuso da sei anni non è latenza: %q", got)
	}

	// `--data`, dove c'è, resta quello che comanda: l'anno è il ripiego.
	misto := map[string]string{"anno": "2019", "data": "2026-09-16"}
	if got := latenzaHint(nil, "ddl", misto, now, m); got == "" {
		t.Error("con --data presente la finestra è quella, non l'anno")
	}

	// E il ripiego vale solo dove `--data` non esiste. Su `ddl` l'anno accanto
	// a `--data` c'è comunque, e prenderlo faceva uscire l'avviso su
	// `--anno 2026 --numero 99999`: un anno pubblicato quasi per intero, dove
	// il vuoto è un vuoto.
	if got := latenzaHint(nil, "ddl", map[string]string{"anno": "2026"}, now, misuraAl("2026-09-14", now)); got != "" {
		t.Errorf("su un archivio con --data l'anno non deve fare da finestra: %q", got)
	}
}

// La sonda guarda l'anno in corso e, se è vuoto, quelli prima: un archivio molto
// indietro a gennaio ha il record più recente a dicembre, e fermarsi all'anno in
// corso perderebbe la cifra proprio a cavallo d'anno. Ma «anno vuoto» e «anno che
// non ho saputo leggere» autorizzano cose diverse: solo il primo lascia guardare
// più indietro, perché il secondo darebbe la frontiera di un anno più vecchio
// spacciandola per l'ultima.
func TestMassimoDellaPagina(t *testing.T) {
	cases := []struct {
		name         string
		date         []string
		troncato     bool
		wantMax      string
		wantEsaurito bool
	}{
		{"ordine decrescente: la prima riga è la frontiera", []string{"2026-09-08", "2026-07-29"}, false, "2026-09-08", true},
		{"decrescente e troncato: la prima riga basta comunque", []string{"2026-09-14", "2026-09-10"}, true, "2026-09-14", true},
		{"ordine sparso, pagina esaurita: massimo sulle righe lette", []string{"2026-06-25", "2026-02-11", "2026-07-22"}, false, "2026-07-22", true},
		{"ordine sparso e troncato: si rinuncia, e non è un anno vuoto", []string{"2026-01-05", "2026-01-08"}, true, "", false},
		{"anno vuoto letto per intero: si guarda quello prima", nil, false, "", true},
		{"nessuna data ma pagina troncata: non si torna indietro", nil, true, "", false},
	}
	for _, c := range cases {
		max, esaurito := massimoDellaPagina(c.date, c.troncato)
		if max != c.wantMax || esaurito != c.wantEsaurito {
			t.Errorf("%s: (%q, %v), atteso (%q, %v)", c.name, max, esaurito, c.wantMax, c.wantEsaurito)
		}
	}
}

// La sonda si restringe all'anno che le viene chiesto, non sempre a quello in
// corso: è il passo su cui poggia il giro all'indietro.
func TestParamsSondaSegueLAnnoChiesto(t *testing.T) {
	arc := icaro.BySlug("risoluzioni")
	if arc == nil {
		t.Fatal("archivio risoluzioni non trovato")
	}
	got := paramsSonda(*arc, map[string]string{"data": "2027-01-02", "legisl": "18"}, 2026)
	if got["data"] != "2026-01-01:2026-12-31" {
		t.Errorf("la finestra deve seguire l'anno chiesto: %v", got)
	}
	if got["legisl"] != "18" {
		t.Errorf("i filtri strutturali restano: %v", got)
	}
}
