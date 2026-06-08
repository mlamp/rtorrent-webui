package config

import (
	"bufio"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Credentials is a set of user -> bcrypt hash, from inline users and/or htpasswd.
type Credentials struct {
	hashes map[string]string
}

// Credentials builds the verifier from the auth config.
func (a Auth) Credentials() (*Credentials, error) {
	c := &Credentials{hashes: map[string]string{}}
	for _, u := range a.Users {
		if u.Name != "" && u.PasswordHash != "" {
			c.hashes[u.Name] = u.PasswordHash
		}
	}
	if a.HtpasswdFile != "" {
		f, err := os.Open(a.HtpasswdFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if i := strings.IndexByte(line, ':'); i > 0 {
				c.hashes[line[:i]] = line[i+1:]
			}
		}
		if err := s.Err(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Credentials) Verify(user, pass string) bool {
	h, ok := c.hashes[user]
	if !ok {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(pass)) == nil
}

func (c *Credentials) Empty() bool { return len(c.hashes) == 0 }
