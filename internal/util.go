// Package internal provides shared helpers used across gomatrixserverlib packages.
package internal

import (
	"context"
	"math/rand/v2"
	"sort"

	"github.com/sirupsen/logrus"
)

type contextKey string

const ctxValueLogger = contextKey("logger")

// GetLogger retrieves the logrus logger from the supplied context. Always returns a logger,
// even if there wasn't one originally supplied.
func GetLogger(ctx context.Context) *logrus.Entry {
	l := ctx.Value(ctxValueLogger)
	if l == nil {
		return logrus.WithField("context", "missing")
	}
	entry, ok := l.(*logrus.Entry)
	if !ok {
		return logrus.WithField("context", "missing")
	}
	return entry
}

// ContextWithLogger creates a new context, which will use the given logger.
func ContextWithLogger(ctx context.Context, l *logrus.Entry) context.Context {
	return context.WithValue(ctx, ctxValueLogger, l)
}

const alphanumerics = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString generates a pseudo-random alphanumeric string of length n.
func RandomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanumerics[rand.IntN(len(alphanumerics))]
	}
	return string(b)
}

// UniqueStrings turns a list of strings into a sorted list of unique strings.
func UniqueStrings(strings []string) []string {
	if len(strings) == 0 {
		return strings
	}
	sort.Strings(strings)
	j := 0
	for i := 1; i < len(strings); i++ {
		if strings[j] != strings[i] {
			j++
			strings[j] = strings[i]
		}
	}
	return strings[:j+1]
}
