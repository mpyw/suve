package version

// isDigitChar reports whether c is an ASCII digit (0-9).
//
//declscope:package // the grammars and the parse engine classify specifier characters with it
func isDigitChar(c byte) bool {
	return c >= '0' && c <= '9'
}

// isLetterChar reports whether c is an ASCII letter (a-z, A-Z).
//
//declscope:package // the grammars and the parse engine classify specifier characters with it
func isLetterChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
