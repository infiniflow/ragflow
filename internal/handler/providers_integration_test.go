//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"
	"ragflow/internal/service"
)

func TestMinerUInstancePersistsLongCredentials_Integration(t *testing.T) {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN unset: this test needs a throwaway MySQL database")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL connection pool: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close MySQL connection pool: %v", err)
		}
	})
	tables, err := db.Migrator().GetTables()
	if err != nil {
		t.Fatalf("list MySQL tables: %v", err)
	}
	if len(tables) != 0 {
		t.Fatal("this test needs an empty throwaway MySQL database")
	}
	useProviderHandlerTestDB(t, db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(permission.WithIdentity(c.Request.Context(), "user-1", "tenant-1"))
	})
	handler := NewProviderHandler(nil, service.NewModelProviderService(), service.NewModelCallService())
	router.PUT("/api/v1/providers/:provider_id_or_name/instances/:instance_id_or_name", handler.AlterProviderInstance)
	router.POST("/api/v1/providers/:provider_id_or_name/instances", handler.CreateProviderInstance)

	for _, schema := range []string{"upgraded", "fresh"} {
		t.Run(schema, func(t *testing.T) {
			t.Cleanup(func() {
				tables, err := db.Migrator().GetTables()
				if err != nil {
					t.Errorf("list fixture tables: %v", err)
					return
				}
				for _, table := range tables {
					if err := db.Migrator().DropTable(table); err != nil {
						t.Errorf("drop fixture table %s: %v", table, err)
					}
				}
			})
			if err := db.AutoMigrate(
				&entity.User{}, &entity.UserTenant{}, &entity.TenantModelProvider{},
				&entity.TenantModelInstance{}, &entity.TenantModel{},
			); err != nil {
				t.Fatalf("create provider tables: %v", err)
			}
			if schema == "upgraded" {
				if err := db.Exec("ALTER TABLE tenant_model_instance MODIFY COLUMN api_key varchar(4096) NOT NULL").Error; err != nil {
					t.Fatalf("set previous API key column type: %v", err)
				}
			}

			const apiServer = "https://mineru.example.test"
			activeStatus := entity.TenantMembershipStatusJoined
			for _, row := range []any{
				&entity.User{ID: "user-1", Nickname: "user", Email: "user-1@test.local", IsAuthenticated: "1", IsActive: "1", IsAnonymous: "0"},
				&entity.UserTenant{ID: "user-tenant-1", UserID: "user-1", TenantID: "tenant-1", Role: "owner", InvitedBy: "user-1", Status: &activeStatus},
				&entity.TenantModelProvider{ID: "provider-1", UserID: "user-1", TenantID: "tenant-1", ProviderName: "MinerU"},
				&entity.TenantModelInstance{ID: "instance-1", ProviderID: "provider-1", InstanceName: "existing", Status: "active", Extra: "{}",
					APIKey: `{"mineru_api_token":"old-token","mineru_apiserver":"https://mineru.example.test","mineru_backend":"pipeline"}`},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatalf("seed %T: %v", row, err)
				}
			}
			if err := testsupport.PrepareEnterprisePermissionFixture(db, "tenant-1", "user-1"); err != nil {
				t.Fatalf("prepare permission fixture: %v", err)
			}
			if schema == "upgraded" {
				if err := dao.RunMigrations(t.Context(), db); err != nil {
					t.Fatalf("run startup migrations: %v", err)
				}
			}

			checkStoredConfig := func(name, token string) {
				t.Helper()
				var instance entity.TenantModelInstance
				if err := db.Take(&instance, "provider_id = ? AND instance_name = ?", "provider-1", name).Error; err != nil {
					t.Fatalf("reload instance %s: %v", name, err)
				}
				cfg := models.MinerUProviderConfigFromAPIKey(instance.APIKey)
				if cfg.AccessToken != token || cfg.APIServer != apiServer || cfg.Backend != "pipeline" {
					t.Fatalf("instance %s did not preserve the token (%d characters) and MinerU settings", name, len(token))
				}
			}
			send := func(method, path, name string, config map[string]string) {
				t.Helper()
				payload, err := json.Marshal(map[string]any{
					"instance_name": name,
					"api_key":       config,
					"model_info":    []service.CreateInstanceModelInfo{},
				})
				if err != nil {
					t.Fatalf("marshal request: %v", err)
				}
				req := httptest.NewRequest(method, path, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				response := decodeProviderHandlerResponse(t, recorder)
				if recorder.Code != http.StatusOK || response["code"] != float64(common.CodeSuccess) {
					t.Fatalf("%s %s returned HTTP %d: %s", method, path, recorder.Code, recorder.Body.String())
				}
			}

			const instancesPath = "/api/v1/providers/MinerU/instances"
			checkStoredConfig("existing", "old-token")
			send(http.MethodPut, instancesPath+"/existing", "existing", map[string]string{"mineru_api_token": "short-token"})
			checkStoredConfig("existing", "short-token")

			// Even a 4,096-character token exceeds the previous column width once
			// stored with its MinerU configuration.
			longToken := strings.Repeat("k", 4096)
			send(http.MethodPut, instancesPath+"/existing", "existing", map[string]string{"mineru_api_token": longToken})
			checkStoredConfig("existing", longToken)

			send(http.MethodPost, instancesPath, "created", map[string]string{
				"mineru_api_token": longToken, "mineru_apiserver": apiServer, "mineru_backend": "pipeline",
			})
			checkStoredConfig("created", longToken)
		})
	}
}
