// Copyright 2026 Derik Parkinson and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSendInputBytes = 25 << 20
const maxSendMIMEBytes = 35 << 20

func parseSendAddresses(values []string) ([]*mail.Address, error) {
	var result []*mail.Address
	for _, value := range values {
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("email addresses must not contain control characters")
		}
		addresses, err := mail.ParseAddressList(value)
		if err != nil {
			return nil, fmt.Errorf("invalid email address: %w", err)
		}
		for _, address := range addresses {
			if !strings.Contains(address.Address, "@") {
				return nil, fmt.Errorf("email address must include a domain")
			}
		}
		result = append(result, addresses...)
	}
	return result, nil
}

func writeSendAddresses(out *bytes.Buffer, name string, values []string) (int, error) {
	addresses, err := parseSendAddresses(values)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	for i, address := range addresses {
		if i == 0 {
			fmt.Fprintf(out, "%s: %s", name, address.String())
		} else {
			fmt.Fprintf(out, ",\r\n %s", address.String())
		}
	}
	if len(addresses) > 0 {
		out.WriteString("\r\n")
	}
	return len(addresses), nil
}

func composeSendMessage(opts sendOptions, stdin io.Reader) ([]byte, error) {
	var out bytes.Buffer
	count, err := writeSendAddresses(&out, "From", []string{opts.from})
	if err != nil || count != 1 {
		return nil, fmt.Errorf("--from must be exactly one valid email address")
	}
	recipients := 0
	for _, header := range []struct {
		name   string
		values []string
	}{
		{"To", opts.to}, {"Cc", opts.cc}, {"Bcc", opts.bcc},
	} {
		n, err := writeSendAddresses(&out, header.name, header.values)
		if err != nil {
			return nil, err
		}
		recipients += n
	}
	if recipients == 0 {
		return nil, fmt.Errorf("at least one --to, --cc or --bcc recipient is required")
	}
	if strings.ContainsAny(opts.subject, "\r\n\x00") || !utf8.ValidString(opts.subject) {
		return nil, fmt.Errorf("subject must be valid UTF-8 without CR, LF or NUL")
	}
	// Always encode and fold, including long ASCII subjects.
	encodedSubject := mime.BEncoding.Encode("UTF-8", opts.subject)
	if len(opts.subject) > 70 && encodedSubject == opts.subject {
		encodedSubject = encodeSendSubject(opts.subject)
	}
	fmt.Fprintf(&out, "Subject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\n",
		strings.ReplaceAll(encodedSubject, "?= =?", "?=\r\n =?"), time.Now().Format(time.RFC1123Z))
	body, err := readSendBody(opts, stdin)
	if err != nil {
		return nil, err
	}
	if err := writeSendContent(&out, body, opts.attachments); err != nil {
		return nil, err
	}
	if out.Len() > maxSendMIMEBytes {
		return nil, fmt.Errorf("encoded message exceeds 35 MiB")
	}
	return out.Bytes(), nil
}

func encodeSendSubject(subject string) string {
	var words []string
	for len(subject) > 0 {
		n := min(42, len(subject)) // This helper is only used for ASCII subjects.
		words = append(words, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(subject[:n]))+"?=")
		subject = subject[n:]
	}
	return strings.Join(words, "\r\n ")
}

func readSendBody(opts sendOptions, stdin io.Reader) ([]byte, error) {
	body := []byte(opts.body)
	var err error
	switch opts.bodyFile {
	case "":
	case "-":
		body, err = io.ReadAll(io.LimitReader(stdin, maxSendInputBytes+1))
	default:
		body, err = readSendFile(opts.bodyFile, maxSendInputBytes)
	}
	if err != nil {
		return nil, err
	}
	if len(body) > maxSendInputBytes {
		return nil, fmt.Errorf("body exceeds 25 MiB")
	}
	if !utf8.Valid(body) {
		return nil, fmt.Errorf("body must be valid UTF-8")
	}
	return body, nil
}

func readSendFile(path string, limit int) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > int64(limit) {
		return nil, fmt.Errorf("message input exceeds 25 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err == nil && len(data) > limit {
		err = fmt.Errorf("message input exceeds 25 MiB")
	}
	return data, err
}

func writeSendText(out io.Writer, body []byte) error {
	w := quotedprintable.NewWriter(out)
	if _, err := w.Write(body); err != nil {
		return err
	}
	return w.Close()
}

func writeSendContent(out *bytes.Buffer, body []byte, paths []string) error {
	if len(paths) == 0 {
		out.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		return writeSendText(out, body)
	}
	w := multipart.NewWriter(out)
	fmt.Fprintf(out, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", w.Boundary())
	part, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	if err := writeSendText(part, body); err != nil {
		return err
	}
	remaining := maxSendInputBytes - len(body)
	for _, path := range paths {
		data, err := readSendFile(path, remaining)
		if err != nil {
			return err
		}
		remaining -= len(data)
		if err := writeSendAttachment(w, filepath.Base(path), data); err != nil {
			return err
		}
	}
	return w.Close()
}

func writeSendAttachment(w *multipart.Writer, name string, data []byte) error {
	if strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("invalid attachment filename")
	}
	part, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"application/octet-stream"},
		"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": name})},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 0 {
		n := min(76, len(encoded))
		if _, err := fmt.Fprintf(part, "%s\r\n", encoded[:n]); err != nil {
			return err
		}
		encoded = encoded[n:]
	}
	return nil
}
