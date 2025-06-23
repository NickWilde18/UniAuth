# 整体系统架构图
- Django主服务：继续负责SSO认证、Session管理和扣费
- UniAuth服务：专注于权限判断和用户组查询
- Redis：共享Session存储
- 微服务：通过Redis获取用户身份，调用UniAuth进行权限判断
```mermaid
graph TB
    subgraph "用户入口"
        User[用户]
        APIClient[API客户端]
        SSO[外部SSO系统]
    end
    
    subgraph "Django主服务"
        DjangoAuth[SSO认证处理]
        DjangoSession[Session管理]
        BillingService[扣费服务]
        DjangoAPI[业务API]
        QuotaDB[(配额池数据库)]
    end
    
    subgraph "UniAuth统一鉴权服务"
        CasbinEngine[Casbin权限引擎]
        AuthAPI[权限判断API]
        GroupAPI[用户组查询API]
        PolicyDB[(策略数据库<br/>SQLite)]
    end
    
    subgraph "微服务集群"
        MS1[微服务1<br/>Go]
        MS2[微服务2<br/>Python]
        KBService[知识库服务]
    end
    
    subgraph "共享存储"
        Redis[(Redis<br/>Session存储)]
    end
    
    %% 认证流程
    User --> SSO
    SSO --> DjangoAuth
    DjangoAuth --> DjangoSession
    DjangoSession --> Redis
    
    %% API Key流程
    APIClient -->|API Key| AuthAPI
    
    %% 权限查询流程
    DjangoAPI -->|1.检查权限| AuthAPI
    DjangoAPI -->|2.查询用户组| GroupAPI
    GroupAPI -->|返回组和配额池| DjangoAPI
    DjangoAPI -->|3.扣费| BillingService
    BillingService --> QuotaDB
    
    %% 微服务流程
    MS1 -->|获取Session| Redis
    MS1 -->|权限判断| AuthAPI
    MS2 -->|获取Session| Redis
    MS2 -->|权限判断| AuthAPI
    KBService -->|获取Session| Redis
    KBService -->|权限判断| AuthAPI
    
    %% 数据存储
    CasbinEngine --> PolicyDB
    AuthAPI --> CasbinEngine
    GroupAPI --> CasbinEngine
    
    %% 样式
    style DjangoAuth fill:#f9f,stroke:#333,stroke-width:4px
    style Redis fill:#f96,stroke:#333,stroke-width:4px
    style CasbinEngine fill:#9f9,stroke:#333,stroke-width:4px
    style BillingService fill:#99f,stroke:#333,stroke-width:4px
```

# 详细数据流程图
- 认证流程：用户通过SSO登录，Django存储Session到Redis
- 模型调用流程：权限检查→查询用户组→扣费→返回结果
- 微服务访问流程：从Redis获取身份→权限检查→执行业务
- API Key调用流程：将API Key映射为特殊UPN进行权限控制
```mermaid
sequenceDiagram
    participant User as 用户
    participant Django as Django主服务
    participant Redis as Redis
    participant UniAuth as UniAuth服务
    participant MS as 微服务
    
    rect rgb(230, 240, 255)
        Note over User,Django: 认证流程（保持不变）
        User->>Django: SSO登录
        Django->>Django: 验证SSO Token
        Django->>Redis: 存储Session<br/>{upn, name, email}
        Django->>User: 返回Session ID
    end
    
    rect rgb(255, 240, 230)
        Note over User,UniAuth: 使用AI模型（带扣费）
        User->>Django: 调用模型API<br/>(Session ID)
        Django->>Redis: 获取UPN
        Django->>UniAuth: 1. 检查权限<br/>{upn, models, gpt-4, use}
        UniAuth-->>Django: {allowed: true}
        Django->>UniAuth: 2. 查询用户组<br/>/user/{upn}/quota-pool
        UniAuth-->>Django: {primaryGroup: "group-student",<br/>quotaPool: "student-pool"}
        Django->>Django: 3. 调用AI模型
        Django->>Django: 4. 从student-pool扣费
        Django->>User: 返回结果+扣费信息
    end
    
    rect rgb(230, 255, 230)
        Note over MS,UniAuth: 微服务访问知识库
        User->>MS: 访问知识库<br/>(Session ID)
        MS->>Redis: 获取Session数据
        Redis-->>MS: {upn: "user@link.cuhk.edu.cn"}
        MS->>UniAuth: 检查权限<br/>{upn, kb, kb-123, read}
        UniAuth-->>MS: {allowed: true}
        MS->>MS: 返回知识库内容
        MS->>User: 返回数据
    end
    
    rect rgb(255, 255, 230)
        Note over User,UniAuth: API Key调用
        User->>Django: API调用<br/>(API Key: sk-xxxxx)
        Django->>UniAuth: 检查权限<br/>{upn: "api:sk-xxxxx",<br/>api, /v1/chat, POST}
        UniAuth-->>Django: {allowed: true}
        Django->>UniAuth: 查询绑定账号<br/>{upn: "api:sk-xxxxx"}
        UniAuth-->>Django: {real_upn: "user@link.cuhk.edu.cn"}
        Django->>Django: 处理请求并扣费
        Django->>User: 返回结果
    end
```

# 权限模型结构图
- 用户只能属于一个基础组（互斥）：student/staff/unlimited/guest
- 每个组的权限独立定义，避免继承带来的混乱
- 每个基础组对应一个配额池，扣费逻辑清晰

- 知识库角色：admin/editor/viewer
- 默认权限：继承知识库级别的权限
- 特殊权限：可以针对特定文档模式设置allow/deny
    - 如：viewer可以读公开文档，但不能读私密文档
```mermaid
graph TB
    subgraph "用户与基础组（互斥）"
        Alice["Alice<br/>alice@link.cuhk.edu.cn"]
        Bob["Bob<br/>bob@temp.com"]
        Charlie["Charlie<br/>charlie@staff.cuhk.edu.cn"]
        
        GS[group-student<br/>学生组]
        GST[group-staff<br/>教职工组]
        GU[group-unlimited<br/>无限制组]
        GG[group-guest<br/>访客组]
        
        Alice -->|手动升级| GST
        Bob --> GG
        Charlie -->|域名匹配| GST
    end
    
    subgraph "API Key 绑定"
        SK1["API Key<br/>sk-alice-proj1"]
        SK2["API Key<br/>sk-alice-proj2"]
        SK3["API Key<br/>sk-bob-dev"]
        
        SK1 -->|绑定| Alice
        SK2 -->|绑定| Alice
        SK3 -->|绑定| Bob
        
        Note1["使用API Key时：<br/>1. 查找绑定的用户<br/>2. 使用该用户的权限<br/>3. 从该用户的配额池扣费"]
    end
    
    subgraph "基础组权限（独立定义）"
        GST --> PGST["✓ GPT-3.5/4<br/>✓ Claude全系列<br/>✓ Llama全系列<br/>💰 staff-pool"]
        GS --> PGS["✓ GPT-3.5<br/>✓ Claude Instant<br/>✓ Llama-13b<br/>💰 student-pool"]
        GU --> PGU["✓ 所有模型<br/>💰 unlimited-pool"]
        GG --> PGG["✓ GPT-3.5<br/>💰 guest-pool"]
    end
    
    style Alice fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    style SK1 fill:#fff3e0,stroke:#e65100,stroke-width:2px
    style SK2 fill:#fff3e0,stroke:#e65100,stroke-width:2px
    style Note1 fill:#f5f5f5,stroke:#616161,stroke-width:1px,stroke-dasharray: 5 5
```
```mermaid
graph TB
    subgraph "知识库权限体系"
        KB1["知识库 kb001"]
        
        subgraph "知识库角色"
            KBA["kb-001-admin<br/>管理员"]
            KBE["kb-001-editor<br/>编辑者"]
            KBV["kb-001-viewer<br/>查看者"]
        end
        
        KB1 --> KBA
        KB1 --> KBE
        KB1 --> KBV
    end
    
    subgraph "文档级别权限"
        subgraph "kb001 文档"
            D1["doc-public-001<br/>公开文档"]
            D2["doc-public-002<br/>公开文档"]
            D3["doc-private-001<br/>私密文档"]
            D4["doc-private-002<br/>私密文档"]
            D5["doc-normal-001<br/>普通文档"]
        end
        
        KBA -->|"✓ 读/写/删除<br/>所有文档"| D1
        KBA --> D2
        KBA --> D3
        KBA --> D4
        KBA --> D5
        
        KBE -->|"✓ 读/写<br/>所有文档"| D1
        KBE --> D2
        KBE --> D3
        KBE --> D4
        KBE --> D5
        
        KBV -->|"✓ 读取<br/>公开文档"| D1
        KBV --> D2
        KBV -->|"❌ 禁止读取<br/>私密文档"| D3
        KBV --> D4
        KBV -->|"✓ 读取<br/>普通文档"| D5
    end
    
    subgraph "用户分配"
        U1["Alice"] -->|分配| KBA
        U2["Charlie"] -->|分配| KBE
        U3["Bob"] -->|分配| KBV
    end
    
    style D3 fill:#ffebee,stroke:#c62828,stroke-width:2px
    style D4 fill:#ffebee,stroke:#c62828,stroke-width:2px
    style KBA fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px
    style KBE fill:#fff9c4,stroke:#f57f17,stroke-width:2px
    style KBV fill:#e1f5fe,stroke:#01579b,stroke-width:2px
```

# 权限流转示意图
```mermaid
graph LR
    subgraph "用户身份"
        USER["用户 UPN<br/>alice\@link.cuhk.edu.cn"]
        APIKEY["API Key<br/>sk-basic-xxxxx"]
    end
    
    subgraph "用户组分配"
        USER --> GS["学生组<br/>group-student"]
        USER --> GKB["知识库管理员<br/>kb-kb001-admin"]
        APIKEY --> GAPI["API基础组<br/>group-api-basic"]
    end
    
    subgraph "权限映射"
        GS --> QS["配额池<br/>student-pool<br/>💰 $100/月"]
        GS --> MS["模型权限<br/>✓ GPT-4o<br/>✓ Qwen3-235B-A22B<br/>❌ GPT-4.1"]
        
        GKB --> KBP["知识库权限<br/>kb001: 完全控制<br/>- 读取/写入/删除<br/>- 成员管理"]
        
        GAPI --> QAPI["配额池<br/>绑定用户"]
        GAPI --> MAPI["API权限<br/>✓ /v1/chat<br/>✓ /v1/embeddings<br/>❌ /admin/*"]
    end
    
    subgraph "扣费决策"
        QS --> BILL1["调用GPT-4o<br/>从student-pool扣费"]
        QAPI --> BILL2["API调用<br/>从绑定账户扣费"]
    end
    
    style USER fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    style APIKEY fill:#fff3e0,stroke:#e65100,stroke-width:2px
    style QS fill:#ffebee,stroke:#b71c1c,stroke-width:2px
    style QAPI fill:#ffebee,stroke:#b71c1c,stroke-width:2px
```
