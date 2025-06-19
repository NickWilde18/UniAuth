package main

import (
	"fmt"
	"log"

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
	//enforcer.EnableCache(true)

	// 4. 载入策略
	if err := enforcer.LoadPolicy(); err != nil {
		log.Fatalf("load policy: %v", err)
	}

	// 5. 写入一些初始化策略
	initPolicies(enforcer)

	// 6. 试验证
	testCases := [][]string{
		{"alice", "/api/user", "GET"},
		{"bob", "/api/user", "GET"},
		{"bob", "/api/article", "POST"},
		{"eve", "/api/data", "GET"},
	}

	for _, c := range testCases {
		ok, _ := enforcer.Enforce(c[0], c[1], c[2])
		fmt.Printf("%-5s -> %-12s %-4s : %v\n", c[0], c[1], c[2], ok)
	}
}

func initPolicies(e *casbin.Enforcer) {
	if ok, _ := e.Enforce("alice", "/api/user", "GET"); ok {
		return
	}
	if _, err := e.AddPolicy("admin", "/api/user", "GET"); err != nil {
		log.Fatalf("add policy: %v", err)
	}
	if _, err := e.AddPolicy("editor", "/api/article", "POST"); err != nil {
		log.Fatalf("add policy: %v", err)

	}
	if _, err := e.AddPolicy("viewer", "/api/data", "GET"); err != nil {
		log.Fatalf("add policy: %v", err)
	}

	if _, err := e.AddRoleForUser("alice", "admin"); err != nil {
		log.Fatalf("add role: %v", err)
	}
	if _, err := e.AddRoleForUser("bob", "editor"); err != nil {
		log.Fatalf("add role: %v", err)
	}
	if _, err := e.AddRoleForUser("eve", "viewer"); err != nil {
		log.Fatalf("add role: %v", err)
	}

	if err := e.SavePolicy(); err != nil {
		log.Fatalf("save policy: %v", err)
	}
}
