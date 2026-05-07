// stub i18n for the chainsawed prototype -- no localisation, just
// pass strings through. matches the API surface of the upstream
// `i18n` package as used inside the tree.

package i18n

// G translates the given message. No-op in the prototype.
func G(msgid string) string {
	return msgid
}

// NG translates a message with plural form. Returns msgid for n==1,
// msgidPlural otherwise. The actual numeric substitution is left to
// the caller's fmt.Sprintf, matching the upstream API.
func NG(msgid, msgidPlural string, n int) string {
	if n == 1 {
		return msgid
	}
	return msgidPlural
}

// Bind is a no-op. Upstream binds the locale; we don't.
func Bind() {}
