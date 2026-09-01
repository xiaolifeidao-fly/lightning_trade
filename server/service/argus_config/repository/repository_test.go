package repository

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestModelsDeclareTablesAndKeyUniqueIndexes(t *testing.T) {
	cache := &sync.Map{}
	for _, model := range []any{
		&ArgusInstance{},
		&ArgusConfigVersion{},
		&ArgusConfig{},
		&ArgusAccount{},
		&ArgusAccountRisk{},
		&ArgusMonitorSymbol{},
		&ArgusNotification{},
		&ArgusRuntimeSession{},
	} {
		parsedSchema, err := schema.Parse(model, cache, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse GORM model %T: %v", model, err)
		}
		if parsedSchema.Table == "" {
			t.Fatalf("model %T has no table name", model)
		}
	}

	assertUniqueIndex(t, cache, &ArgusInstance{}, "idx_argus_instance_key")
	assertUniqueIndex(t, cache, &ArgusConfigVersion{}, PublishedSlotIndexName)
	assertUniqueIndex(t, cache, &ArgusConfigVersion{}, VersionSequenceIndexName)
	assertUniqueIndex(t, cache, &ArgusAccount{}, "idx_argus_account_version_name")
	assertUniqueIndex(t, cache, &ArgusRuntimeSession{}, "idx_argus_runtime_session_account")

	// published 与 version 的唯一性都必须收在实例内，否则一次发布会打翻其他实例。
	assertIndexColumns(t, cache, &ArgusConfigVersion{}, PublishedSlotIndexName, []string{"instance_key", "published_slot"})
	assertIndexColumns(t, cache, &ArgusConfigVersion{}, VersionSequenceIndexName, []string{"instance_key", "version"})
}

func assertIndexColumns(t *testing.T, cache *sync.Map, model any, indexName string, wanted []string) {
	t.Helper()
	parsedSchema, err := schema.Parse(model, cache, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse GORM model %T: %v", model, err)
	}
	index := parsedSchema.ParseIndexes()[indexName]
	columns := make([]string, 0, len(index.Fields))
	for _, field := range index.Fields {
		columns = append(columns, field.DBName)
	}
	if strings.Join(columns, ",") != strings.Join(wanted, ",") {
		t.Fatalf("index %s columns = %v, want %v", indexName, columns, wanted)
	}
}

func TestInstanceScopedQueriesRejectMissingInstanceKey(t *testing.T) {
	repo := &ArgusConfigRepository{}
	if _, err := repo.FindPublished("  "); !errors.Is(err, ErrInstanceKeyRequired) {
		t.Fatalf("FindPublished error = %v, want ErrInstanceKeyRequired", err)
	}
	if _, err := repo.NextVersion(""); !errors.Is(err, ErrInstanceKeyRequired) {
		t.Fatalf("NextVersion error = %v, want ErrInstanceKeyRequired", err)
	}
	if _, err := repo.FindByVersion("", 1); !errors.Is(err, ErrInstanceKeyRequired) {
		t.Fatalf("FindByVersion error = %v, want ErrInstanceKeyRequired", err)
	}
	if _, _, _, _, _, _, _, err := repo.LoadSnapshot("", 1); !errors.Is(err, ErrInstanceKeyRequired) {
		t.Fatalf("LoadSnapshot error = %v, want ErrInstanceKeyRequired", err)
	}
	if _, err := repo.BackfillInstanceKey(""); !errors.Is(err, ErrInstanceKeyRequired) {
		t.Fatalf("BackfillInstanceKey error = %v, want ErrInstanceKeyRequired", err)
	}
}

func assertUniqueIndex(t *testing.T, cache *sync.Map, model any, indexName string) {
	t.Helper()
	parsedSchema, err := schema.Parse(model, cache, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse GORM model %T: %v", model, err)
	}
	index, exists := parsedSchema.ParseIndexes()[indexName]
	if !exists || index.Class != "UNIQUE" {
		t.Fatalf("model %T lacks unique index %q", model, indexName)
	}
}
