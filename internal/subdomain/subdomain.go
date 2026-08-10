package subdomain

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const length = 6
const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

var reserved = map[string]struct{}{
	"www":     {},
	"api":     {},
	"connect": {},
	"dashboard": {},
}

func IsReserved(name string) bool {
	_, ok := reserved[name]
	return ok
}

func Generate(exists func(string) bool) (string, error) {
	for i := 0; i < 20; i++ {
		name, err := randomName()
		if err != nil {
			return "", err
		}
		if IsReserved(name) || exists(name) {
			continue
		}
		return name, nil
	}
	return "", fmt.Errorf("не удалось сгенерировать subdomain")
}

func randomName() (string, error) {
	buf := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))
	for i := range buf {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[n.Int64()]
	}
	return string(buf), nil
}

func Extract(host, baseDomain string) string {
	host = stripPort(host)
	if host == baseDomain {
		return ""
	}

	suffix := "." + baseDomain
	if len(host) > len(suffix) && host[len(host)-len(suffix):] == suffix {
		return host[:len(host)-len(suffix)]
	}

	if len(host) > len(".localhost") && host[len(host)-len(".localhost"):] == ".localhost" {
		return host[:len(host)-len(".localhost")]
	}

	return ""
}

func stripPort(host string) string {
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

func PublicURL(baseDomain, name string) string {
	return "https://" + name + "." + baseDomain
}
