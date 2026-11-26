package api

import (
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"unicode/utf8"
)

// validator кіріс деректерінің қателерін жинайды және бәрін бір хабармен қайтарады.
type validator struct {
	errs map[string]string
}

func (v *validator) add(field, msg string) {
	if v.errs == nil {
		v.errs = map[string]string{}
	}
	if _, exists := v.errs[field]; !exists {
		v.errs[field] = msg
	}
}

func (v *validator) check(cond bool, field, msg string) {
	if !cond {
		v.add(field, msg)
	}
}

func (v *validator) length(field, s string, min, max int) {
	n := utf8.RuneCountInString(s)
	switch {
	case min > 0 && n == 0:
		v.add(field, "міндетті өріс")
	case n < min:
		v.add(field, "тым қысқа")
	case n > max:
		v.add(field, "тым ұзын")
	}
}

func (v *validator) email(field, s string) {
	a, err := mail.ParseAddress(s)
	if s == "" {
		v.add(field, "міндетті өріс")
	} else if err != nil || a.Address != s || !strings.Contains(s, ".") || len(s) > 255 {
		v.add(field, "email форматы қате")
	}
}

func (v *validator) password(field, s string) {
	n := utf8.RuneCountInString(s)
	v.check(n >= 6, field, "құпиясөз кемінде 6 таңба болуы керек")
	v.check(len(s) <= 72, field, "құпиясөз тым ұзын") // bcrypt шектеуі
}

func (v *validator) oneOf(field, s string, allowed ...string) {
	for _, a := range allowed {
		if s == a {
			return
		}
	}
	v.add(field, "рұқсат етілген мәндер: "+strings.Join(allowed, ", "))
}

func (v *validator) valid() bool { return len(v.errs) == 0 }

// ok қате болса 400 жауабын жазып, false қайтарады.
// Формат: { "error": "full_name: міндетті өріс; email: ...", "fields": {...} }
func (v *validator) ok(w http.ResponseWriter) bool {
	if v.valid() {
		return true
	}
	keys := make([]string, 0, len(v.errs))
	for k := range v.errs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + v.errs[k]
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":  strings.Join(parts, "; "),
		"fields": v.errs,
	})
	return false
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
