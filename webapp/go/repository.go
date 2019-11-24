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

func (r *Repository) getUser(idx int64) (*User, error) {
	var user User
	key := fmt.Sprintf("user_%d", idx)

	v, err := r.cache.Get(key)
	if err == nil {
		err = json.Unmarshal(v, &user)
		if err == nil {
			return &user, nil
		}
	}
	log.Print(err)

	err = r.dbx.Get(&user, "SELECT * FROM `users` WHERE `id` = ?", idx)
	if err != nil {
		return nil, err
	}

	err = r.setUser(&user)
	if err != nil {
		log.Print(err)
	}
	return &user, nil
}
