package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/other/anac-pl/internal/client"
	"github.com/mvanhorn/printing-press-library/library/other/anac-pl/internal/config"
)

// clientVerso costruisce un client puntato al server di prova, senza cache né
// attese del rate limiter.
func clientVerso(url string) *client.Client {
	c := client.New(&config.Config{BaseURL: url}, 5*time.Second, 1000)
	c.NoCache = true
	return c
}

// TestFetchFullTextPaginaATokenE rispetta le tre condizioni di stop: token
// assente, pagina più corta di size, pagina senza id nuovi.
func TestFetchFullTextPaginaAToken(t *testing.T) {
	var richieste []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := map[string]string{}
		for k, v := range r.URL.Query() {
			q[k] = v[0]
		}
		richieste = append(richieste, q)
		switch q["tokenPaginazione"] {
		case "":
			fmt.Fprint(w, `{"count":7,"content":[{"idAvviso":"a"},{"idAvviso":"b"}],"lastPaginationToken":"T1"}`)
		case "T1":
			// "b" è già vista: va scartata, non conta come pagina corta
			fmt.Fprint(w, `{"count":7,"content":[{"idAvviso":"b"},{"idAvviso":"c"}],"lastPaginationToken":"T2"}`)
		default:
			fmt.Fprint(w, `{"count":7,"content":[{"idAvviso":"d"},{"idAvviso":"e"}],"lastPaginationToken":""}`)
		}
	}))
	defer srv.Close()

	items, total, _, err := fetchFullText(context.Background(), clientVerso(srv.URL), map[string]string{"size": "2", "keywords": "x"}, 5)
	if err != nil {
		t.Fatalf("fetchFullText: %v", err)
	}
	if total != 7 {
		t.Errorf("count = %d; atteso 7", total)
	}
	var ids []string
	for _, raw := range items {
		var m struct {
			IDAvviso string `json:"idAvviso"`
		}
		_ = json.Unmarshal(raw, &m)
		ids = append(ids, m.IDAvviso)
	}
	if fmt.Sprint(ids) != "[a b c d e]" {
		t.Errorf("id = %v; attesi [a b c d e] senza duplicati", ids)
	}
	// tre pagine chieste, la quarta no: la terza non ha token
	if len(richieste) != 3 {
		t.Fatalf("richieste = %d; attese 3", len(richieste))
	}
	if richieste[0]["tokenPaginazione"] != "" || richieste[0]["direzionePaginazione"] != "" {
		t.Errorf("la prima pagina non deve portare token: %v", richieste[0])
	}
	if richieste[1]["tokenPaginazione"] != "T1" || richieste[1]["direzionePaginazione"] != "AVANTI" {
		t.Errorf("seconda pagina senza token AVANTI: %v", richieste[1])
	}
	// il numero di pagina non viene mai mandato: il servizio lo ignora
	for _, q := range richieste {
		if _, c := q["page"]; c {
			t.Errorf("mandato page=%v: il servizio lo ignora", q["page"])
		}
	}
}

// TestFetchFullTextPaginaCorta ferma lo scorrimento quando il servizio
// restituisce meno di size risultati, anche se dichiara un token.
func TestFetchFullTextPaginaCorta(t *testing.T) {
	chiamate := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chiamate++
		fmt.Fprint(w, `{"count":1,"content":[{"idAvviso":"a"}],"lastPaginationToken":"T1"}`)
	}))
	defer srv.Close()

	items, _, _, err := fetchFullText(context.Background(), clientVerso(srv.URL), map[string]string{"size": "20"}, 4)
	if err != nil {
		t.Fatalf("fetchFullText: %v", err)
	}
	if chiamate != 1 || len(items) != 1 {
		t.Errorf("chiamate = %d, item = %d; attesi 1 e 1", chiamate, len(items))
	}
}

// TestFetchFullTextJSONMalformato: una pagina che risponde 200 con un corpo non
// decodificabile deve dare errore. Prima ne usciva un risultato parziale
// dichiarato come successo, indistinguibile da una paginazione finita bene.
func TestFetchFullTextJSONMalformato(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tokenPaginazione") == "" {
			fmt.Fprint(w, `{"count":9,"content":[{"idAvviso":"a"},{"idAvviso":"b"}],"lastPaginationToken":"T1"}`)
			return
		}
		fmt.Fprint(w, `{"count":9,"content":[{"idAvviso":`)
	}))
	defer srv.Close()

	items, _, fetched, err := fetchFullText(context.Background(), clientVerso(srv.URL), map[string]string{"size": "2"}, 3)
	if err == nil {
		t.Fatalf("attesa un'uscita in errore, ottenuti %d avvisi e %d pagine senza errore", len(items), fetched)
	}
	if fetched != 1 {
		t.Errorf("pagine scaricate = %d, attesa 1 (solo quella valida)", fetched)
	}
}
