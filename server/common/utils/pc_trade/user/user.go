package user

type User struct {
	Cookie          string
	Token           string
	SentryRelease   string
	SentryPublicKey string
}

func NewUser(cookie, token, sentryRelease, sentryPublicKey string) *User {
	return &User{Cookie: cookie, Token: token, SentryRelease: sentryRelease, SentryPublicKey: sentryPublicKey}
}
