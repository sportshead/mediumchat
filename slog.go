package main

import (
	"log/slog"
)

func slogTag(tag string) slog.Attr {
	return slog.String("tag", tag)
}

func slogError(err error) slog.Attr {
	return slog.Any("error", err)
}
