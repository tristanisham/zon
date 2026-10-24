package zon

type tokenKind int

const (
	tokenEOF         tokenKind = iota
	tokenDotBrace              // .{
	tokenLBrace                // { (after a dot separated by trivia)
	tokenRBrace                // }
	tokenEquals                // =
	tokenComma                 // ,
	tokenDot                   // .  (precedes a field name or an enum literal)
	tokenMinus                 // -
	tokenIdent                 // bare identifier or keyword
	tokenQuotedIdent           // @"quoted" identifier
	tokenChar                  // character literal as an integer codepoint
	tokenString                // string literal (value holds the decoded contents)
	tokenInt                   // integer literal (value holds the raw text)
	tokenFloat                 // floating-point literal (value holds the raw text)
)

// token is a single lexical unit. line and col are 1-based and point at the
// first byte of the token.
type token struct {
	kind  tokenKind
	value string
	line  int
	col   int
}
