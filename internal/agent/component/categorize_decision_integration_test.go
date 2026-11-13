//go:build integration

package component

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/server"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestCategorizeDecisionConfiguredModel reads an added SystemOne model from the
// configured database and executes the registered workflow node against it.
// RAGFLOW_DECISION_LIVE=1 opts into paid calls; RAGFLOW_DECISION_LIVE_MODEL may
// select a particular tenant_model.id when several models have been added.
func TestCategorizeDecisionConfiguredModel(t *testing.T) {
	if os.Getenv("RAGFLOW_DECISION_LIVE") != "1" {
		t.Skip("set RAGFLOW_DECISION_LIVE=1 to call the configured SystemOne model")
	}
	configPath := os.Getenv("RAGFLOW_DECISION_LIVE_CONFIG")
	if configPath == "" {
		configPath = filepath.Join("..", "..", "..", "conf", "service_conf.yaml")
	}
	if err := server.Init(configPath); err != nil {
		t.Fatal("load service configuration:", err)
	}
	dao.GetModelProviderManager()
	cfg := server.GetConfig().GetMySQLConfig()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&timeout=10s", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DatabaseName, cfg.Charset)
	// Open a connection directly: InitDB also performs migrations and seeding.
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("connect to configured database:", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var selected struct{ ID, TenantID string }
	query := db.WithContext(t.Context()).Table("tenant_model AS m").Select("m.id, p.tenant_id").
		Joins("JOIN tenant_model_provider AS p ON p.id = m.provider_id").
		Joins("JOIN tenant_model_instance AS i ON i.id = m.instance_id AND i.provider_id = p.id").
		Where("p.provider_name = ? AND m.status = ? AND i.status = ? AND m.model_type & ? <> 0", "SystemOne", "active", "active", entity.ModelTypeDecision)
	if modelID := os.Getenv("RAGFLOW_DECISION_LIVE_MODEL"); modelID != "" {
		query = query.Where("m.id = ?", modelID)
	}
	if err := query.Order("m.id").Take(&selected).Error; err != nil {
		t.Fatal("find active SystemOne model:", err)
	}
	for _, tc := range []struct{ name, query, want string }{
		{"billing", "I was charged twice for my subscription. Please refund the duplicate payment.", "Billing"},
		{"technical support", "我登录时一直报错，无法使用你们的软件，请帮我排查。", "Technical support"},
		{"other", "What is the weather tomorrow?", "Other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New("Categorize", map[string]any{
				"llm_id": selected.ID, "query": "sys.query",
				"items": []any{
					map[string]any{"name": "Billing", "description": "Payments, invoices, charges and refunds", "uuid": "billing-route"},
					map[string]any{"name": "Technical support", "description": "Troubleshoot software errors and login problems", "uuid": "support-route"},
					map[string]any{"name": "Other", "description": "Requests unrelated to billing or software support", "uuid": "other-route"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			state := canvas.NewCanvasState("decision-live", "categorize-live")
			state.Sys["tenant_id"], state.Sys["query"] = selected.TenantID, tc.query
			out, err := c.Invoke(canvas.WithState(t.Context(), state), db, nil)
			if err != nil {
				t.Fatal(err)
			}
			routes := map[string]string{"Billing": "billing-route", "Technical support": "support-route", "Other": "other-route"}
			if out["category"] != tc.want || !reflect.DeepEqual(out["_next"], []string{routes[tc.want]}) {
				t.Fatalf("unexpected classification: %v; want %s", out, tc.want)
			}
			t.Logf("category=%s route=%v", out["category"], out["_next"])
		})
	}
}
