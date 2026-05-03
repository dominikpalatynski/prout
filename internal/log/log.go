package log

import (
	"context"
	"log/slog"
	"os"
)

func Init() error {
	textLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	textLogger.InfoContext(context.Background(), "TextHandler", "Name", "Leapcell")
	return nil
}
