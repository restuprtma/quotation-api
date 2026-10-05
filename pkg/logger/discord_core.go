package logger

import (
	"strings"
	"time"

	"venturo-skeleton-go/pkg/discord"

	"go.uber.org/zap/zapcore"
)

// discordSafeFieldKeys is deliberately smaller than the set of fields written
// to the primary Zap sink. Discord is an external alert channel, so it must
// receive only bounded operational metadata and never request/provider text,
// credentials, identifiers, or arbitrary errors.
var discordSafeFieldKeys = map[string]struct{}{
	"duration_ms":         {},
	"calls_used":          {},
	"calls_max":           {},
	"input_tokens":        {},
	"output_tokens":       {},
	"attempt":             {},
	"http_status":         {},
	"retry_after_seconds": {},
}

// discordCore is a zapcore.Core that forwards entries at or above minLevel
// to a Discord webhook via a rate-limited notifier. It does not encode or
// write the entry anywhere else — pair with NewTee so the primary core
// still writes to stdout/file.
type discordCore struct {
	minLevel zapcore.Level
	notifier *discord.Notifier
	fields   []zapcore.Field
}

func newDiscordCore(webhookURL, levelStr string, debounce time.Duration, source string) zapcore.Core {
	lvl := zapcore.WarnLevel
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "error":
		lvl = zapcore.ErrorLevel
	case "warn", "warning", "":
		lvl = zapcore.WarnLevel
	}
	return &discordCore{
		minLevel: lvl,
		notifier: discord.NewNotifier(webhookURL, debounce, source),
	}
}

func (c *discordCore) Enabled(lvl zapcore.Level) bool {
	return lvl >= c.minLevel
}

func (c *discordCore) With(fields []zapcore.Field) zapcore.Core {
	clone := *c
	if len(fields) > 0 {
		merged := make([]zapcore.Field, 0, len(c.fields)+len(fields))
		merged = append(merged, c.fields...)
		merged = append(merged, fields...)
		clone.fields = merged
	}
	return &clone
}

func (c *discordCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *discordCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	allFields := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	allFields = append(allFields, c.fields...)
	allFields = append(allFields, fields...)

	c.notifier.Notify(discordEventForAlert(ent, allFields))
	return nil
}

func discordEventForAlert(ent zapcore.Entry, fields []zapcore.Field) discord.Event {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		if discordSafeField(f) {
			f.AddTo(enc)
		}
	}

	return discord.Event{
		Level: ent.Level.String(),
		// Entry messages are free text and may contain request or provider
		// content. Use a fixed message; detailed data remains only in the
		// primary, access-controlled Zap sink.
		Message: "application alert",
		Caller:  ent.Caller.TrimmedPath(),
		Time:    ent.Time,
		Fields:  enc.Fields,
	}
}

func discordSafeField(field zapcore.Field) bool {
	if _, ok := discordSafeFieldKeys[field.Key]; !ok {
		return false
	}

	// Only scalar Zap encodings are allowed. In particular, ErrorType,
	// ObjectMarshalerType, ReflectType, and StringerType can contain
	// arbitrary free text and are never forwarded.
	switch field.Type {
	case zapcore.BoolType,
		zapcore.Int64Type,
		zapcore.Int32Type,
		zapcore.Int16Type,
		zapcore.Int8Type,
		zapcore.Uint64Type,
		zapcore.Uint32Type,
		zapcore.Uint16Type,
		zapcore.Uint8Type:
		return true
	default:
		return false
	}
}

func (c *discordCore) Sync() error { return nil }
