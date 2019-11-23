package main

import (
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"

	"github.com/adelowo/onecache"
	"github.com/adelowo/onecache/memcached"
	"github.com/bradfitz/gomemcache/memcache"
)

type Repository struct {
	dbx   *sqlx.DB
	cache *memcached.MemcachedStore
}

func initRepositories(dbx *sqlx.DB, host string) *Repository {
	return &Repository{
		dbx,
		memcached.NewMemcachedStore(memcache.New(host), ""),
	}
}

func (r *Repository) flush() error {
	return r.cache.Flush()
}

func (r *Repository) setUser(u *User) error {
	v, err := json.Marshal(u)
	if err != nil {
		return nil
	}

	return r.cache.Set(fmt.Sprintf("user_%d", u.ID), v, onecache.EXPIRES_DEFAULT)
}

func (r *Repository) getUser(idx int64) (u *User, err error) {
	key := fmt.Sprintf("user_%d", idx)
	if r.cache.Has(key) {
		v, err := r.cache.Get(key)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(v, u)
		return u, err
	}

	var user User
	err = r.dbx.Get(&user, "SELECT * FROM `users` WHERE `id` = ?", idx)
	if err != nil {
		return
	}

	err = r.setUser(&user)
	if err != nil {
		log.Print(err)
	}

	return &user, nil
}
