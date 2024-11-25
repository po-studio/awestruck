package types

type TURNCredentials struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	TTL      int64    `json:"ttl"`
	URLs     []string `json:"urls"`
}
