package repository

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestModelsDeclareTablesAndKeyUniqueIndexes(t *testing.T) {
	cache := &sync.Map{}
	for _, model := range []any{
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

	assertUniqueIndex(t, cache, &ArgusConfigVersion{}, "idx_argus_single_published")
	assertUniqueIndex(t, cache, &ArgusAccount{}, "idx_argus_account_version_name")
	assertUniqueIndex(t, cache, &ArgusRuntimeSession{}, "idx_argus_runtime_session_account")
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
