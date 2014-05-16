// Memcache session support for Gorilla Web Toolkit,
// without Google App Engine dependency.
package gsm

import (
	"encoding/base32"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
	"strings"
)

// NewMemcacheStore returns a new MemcacheStore.
// You need to provide the memcache client and
// an optional prefix for the keys we store
func NewMemcacheStore(client *memcache.Client, keyPrefix string, keyPairs ...[]byte) *MemcacheStore {

	if client == nil {
		panic("Cannot have nil memcache client")
	}

	return &MemcacheStore{
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: 86400 * 30,
		},
		KeyPrefix: keyPrefix,
		Client:    client,
	}
}

// MemcacheStore stores sessions in memcache
//
type MemcacheStore struct {
	Codecs    []securecookie.Codec
	Options   *sessions.Options // default configuration
	Client    *memcache.Client
	KeyPrefix string
}

// MaxLength restricts the maximum length of new sessions to l.
// If l is 0 there is no limit to the size of a session, use with caution.
// The default for a new MemcacheStore is 4096.
func (s *MemcacheStore) MaxLength(l int) {
	for _, c := range s.Codecs {
		if codec, ok := c.(*securecookie.SecureCookie); ok {
			codec.MaxLength(l)
		}
	}
}

// Get returns a session for the given name after adding it to the registry.
//
// See CookieStore.Get().
func (s *MemcacheStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

// New returns a session for the given name without adding it to the registry.
//
// See CookieStore.New().
func (s *MemcacheStore) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(s, name)
	opts := *s.Options
	session.Options = &opts
	session.IsNew = true
	var err error
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if err == nil {
			err = s.load(session)
			if err == nil {
				session.IsNew = false
			}
		}
	}
	return session, err
}

// Save adds a single session to the response.
func (s *MemcacheStore) Save(r *http.Request, w http.ResponseWriter,
	session *sessions.Session) error {
	if session.ID == "" {
		// Because the ID is used in the filename, encode it to
		// use alphanumeric characters only.
		session.ID = strings.TrimRight(
			base32.StdEncoding.EncodeToString(
				securecookie.GenerateRandomKey(32)), "=")
	}
	if err := s.save(session); err != nil {
		return err
	}
	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID,
		s.Codecs...)
	if err != nil {
		return err
	}
	http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

// save writes encoded session.Values using the memcache client
func (s *MemcacheStore) save(session *sessions.Session) error {
	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values,
		s.Codecs...)
	if err != nil {
		return err
	}

	key := s.KeyPrefix + session.ID

	err = s.Client.Set(&memcache.Item{Key: key, Value: []byte(encoded)})
	if err != nil {
		return err
	}

	return nil
}

// load reads a file and decodes its content into session.Values.
func (s *MemcacheStore) load(session *sessions.Session) error {

	key := s.KeyPrefix + session.ID

	it, err := s.Client.Get(key)

	if err = securecookie.DecodeMulti(session.Name(), string(it.Value),
		&session.Values, s.Codecs...); err != nil {
		return err
	}
	return nil
}
