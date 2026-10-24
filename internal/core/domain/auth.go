package domain

type Token struct {
	AccessToken string
	TokenType   string
	ExpiresIn   int
}
