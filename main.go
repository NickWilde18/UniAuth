package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/sqlite" // 替换驱动
	"gorm.io/gorm"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
)

func main() {
	// 1. 连数据库
	db, err := gorm.Open(sqlite.Open("casbin.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}

	// 2. 迁移
	if err := db.AutoMigrate(&gormadapter.CasbinRule{}); err != nil {
		log.Fatalf("auto-migrate: %v", err)
	}

	// 3. 适配器 + Enforcer
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		log.Fatalf("new adapter: %v", err)
	}
	enforcer, err := casbin.NewEnforcer("model/rbac_model.conf", adapter)
	if err != nil {
		log.Fatalf("new enforcer: %v", err)
	}

	// 4. 载入策略
	if err := enforcer.LoadPolicy(); err != nil {
		log.Fatalf("load policy: %v", err)
	}

	// 5. 写入一些初始化策略
	if err := initPoliciesFromCSV(enforcer, "policy.csv"); err != nil {
		log.Fatalf("init policies from csv: %v", err)
		return
	}

	// 6. 试验证
	testCases := [][]string{
		{"user's upn", "models", "qwen3", "use"},
		{"bob", "/api/user", "GET"},
		{"bob", "/api/article", "POST"},
		{"eve", "/api/data", "GET"},
	}

	for _, c := range testCases {
		ok, _ := enforcer.Enforce(c[0], c[1], c[2])
		fmt.Printf("%-5s -> %-12s %-4s : %v\n", c[0], c[1], c[2], ok)
	}
}

func initPoliciesFromCSV(e *casbin.Enforcer, csvFile string) error {
	f, err := os.Open(csvFile)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	lines, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case "p":
			// p, sub, obj, act
			if len(line) < 4 {
				continue
			}
			_, _ = e.AddPolicy(line[1], line[2], line[3])
		case "g":
			// g, user/child, role/parent
			if len(line) < 3 {
				continue
			}
			_, _ = e.AddRoleForUser(line[1], line[2])
		default:
			// 其它类型忽略
		}
	}

	return e.SavePolicy() // 写回 SQLite
}
