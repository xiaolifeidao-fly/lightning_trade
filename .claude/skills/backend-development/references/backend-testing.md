# Backend Testing Strategies (Go)

Go 后端测试方法、框架与质量保障实践。

## Test Pyramid (70-20-10 Rule)

```
        /\
       /E2E\     10% - End-to-End Tests
      /------\
     /Integr.\ 20% - Integration Tests
    /----------\
   /   Unit     \ 70% - Unit Tests
  /--------------\
```

**Rationale:**
- Unit tests: Fast, cheap, isolate bugs quickly
- Integration tests: Verify component interactions
- E2E tests: Expensive, slow, but validate real user flows

## Unit Testing

### Frameworks

- **testing** - 标准库，table-driven tests
- **testify** - 断言与 mock (`github.com/stretchr/testify`)
- **gomock** - 官方 mock 框架 (`go.uber.org/mock`)
- **go-cmp** - 深度比较 (`github.com/google/go-cmp`)

### Table-Driven Tests

```go
func TestCreateUser(t *testing.T) {
    tests := []struct {
        name    string
        input   UserInput
        wantErr bool
        errMsg  string
    }{
        {
            name:  "valid user",
            input: UserInput{Email: "test@example.com", Name: "Test"},
        },
        {
            name:    "duplicate email",
            input:   UserInput{Email: "existing@example.com", Name: "Test"},
            wantErr: true,
            errMsg:  "email already exists",
        },
        {
            name:    "invalid email",
            input:   UserInput{Email: "bad-email", Name: "Test"},
            wantErr: true,
            errMsg:  "invalid email format",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            svc := NewUserService(mockRepo)
            user, err := svc.CreateUser(tt.input)

            if tt.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.input.Email, user.Email)
            assert.NotEmpty(t, user.ID)
        })
    }
}
```

### Mocking with gomock

```go
//go:generate mockgen -source=repository.go -destination=mock_repository.go -package=user

type UserRepository interface {
    Create(ctx context.Context, u *User) error
    FindByEmail(ctx context.Context, email string) (*User, error)
}

func TestCreateUser_SendsWelcomeEmail(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockRepo := NewMockUserRepository(ctrl)
    mockMailer := NewMockMailer(ctrl)

    mockRepo.EXPECT().
        FindByEmail(gomock.Any(), "test@example.com").
        Return(nil, ErrNotFound)
    mockRepo.EXPECT().
        Create(gomock.Any(), gomock.Any()).
        Return(nil)
    mockMailer.EXPECT().
        SendWelcomeEmail("test@example.com").
        Return(nil)

    svc := NewUserService(mockRepo, mockMailer)
    _, err := svc.CreateUser(UserInput{Email: "test@example.com"})
    require.NoError(t, err)
}
```

### Mocking with testify

```go
type MockUserRepo struct {
    mock.Mock
}

func (m *MockUserRepo) Create(ctx context.Context, u *User) error {
    args := m.Called(ctx, u)
    return args.Error(0)
}

func TestCreateUser_HashesPassword(t *testing.T) {
    repo := new(MockUserRepo)
    repo.On("Create", mock.Anything, mock.MatchedBy(func(u *User) bool {
        return u.Password != "plain123" && strings.HasPrefix(u.Password, "$argon2id$")
    })).Return(nil)

    svc := NewUserService(repo)
    _, err := svc.CreateUser(UserInput{Email: "test@example.com", Password: "plain123"})

    require.NoError(t, err)
    repo.AssertExpectations(t)
}
```

## Integration Testing

### HTTP API Tests

```go
func TestUserAPI(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    router := setupRouter(db)
    srv := httptest.NewServer(router)
    defer srv.Close()

    t.Run("POST /api/user returns 200 with code 0", func(t *testing.T) {
        body := `{"username":"testuser","email":"test@example.com"}`
        resp, err := http.Post(srv.URL+"/api/user", "application/json", strings.NewReader(body))
        require.NoError(t, err)
        defer resp.Body.Close()

        assert.Equal(t, http.StatusOK, resp.StatusCode)

        var result map[string]interface{}
        require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
        assert.Equal(t, float64(0), result["code"])

        // Verify database persistence
        var count int
        err = db.QueryRow("SELECT COUNT(*) FROM user_record WHERE email = ?", "test@example.com").Scan(&count)
        require.NoError(t, err)
        assert.Equal(t, 1, count)
    })

    t.Run("POST /api/user returns error for missing username", func(t *testing.T) {
        body := `{"email":"test@example.com"}`
        resp, err := http.Post(srv.URL+"/api/user", "application/json", strings.NewReader(body))
        require.NoError(t, err)
        defer resp.Body.Close()

        var result map[string]interface{}
        require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
        assert.Equal(t, float64(1), result["code"])
    })
}
```

### Database Testing with TestContainers

```go
import (
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()
    ctx := context.Background()

    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "mysql:8.0",
            ExposedPorts: []string{"3306/tcp"},
            Env: map[string]string{
                "MYSQL_ROOT_PASSWORD": "test",
                "MYSQL_DATABASE":      "testdb",
            },
            WaitingFor: wait.ForListeningPort("3306/tcp"),
        },
        Started: true,
    })
    require.NoError(t, err)
    t.Cleanup(func() { container.Terminate(ctx) })

    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "3306")

    dsn := fmt.Sprintf("root:test@tcp(%s:%s)/testdb?charset=utf8mb4&parseTime=True&loc=Local", host, port.Port())
    db, err := sql.Open("mysql", dsn)
    require.NoError(t, err)

    return db
}
```

## Load Testing

### k6

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '2m', target: 100 },
    { duration: '5m', target: 100 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],
  },
};

export default function () {
  const res = http.get('https://api.example.com/users');
  check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 500ms': (r) => r.timings.duration < 500,
  });
  sleep(1);
}
```

### Performance Thresholds

- **Response time:** p95 < 500ms, p99 < 1s
- **Throughput:** 1000+ req/sec (target based on SLA)
- **Error rate:** < 1%
- **Concurrent users:** Test at 2x expected peak

## Database Migration Testing

```go
func TestMigration_V2AddCreatedAt(t *testing.T) {
    db := setupTestDB(t)
    runMigrations(db, "v1")

    _, err := db.Exec("INSERT INTO user_record (id, username, email) VALUES (1, 'testuser', 'test@example.com')")
    require.NoError(t, err)

    runMigrations(db, "v2-add-created-at")

    var user struct {
        ID        int
        Email     string
        Username  string
        CreatedAt time.Time
    }
    err = db.QueryRow("SELECT id, username, email, created_time FROM user_record WHERE id = ?", 1).
        Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt)
    require.NoError(t, err)

    assert.Equal(t, "test@example.com", user.Email)
    assert.False(t, user.CreatedAt.IsZero())
}

func TestMigration_Rollback(t *testing.T) {
    db := setupTestDB(t)
    runMigrations(db, "v2-add-created-at")
    rollbackMigration(db, "v2-add-created-at")

    var count int
    err := db.QueryRow(`
        SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema = DATABASE() AND table_name = 'user_record' AND column_name = 'created_time'
    `).Scan(&count)
    require.NoError(t, err)
    assert.Equal(t, 0, count)
}
```

## Security Testing

### SAST (Static Application Security Testing)

```bash
# gosec - Go security checker
gosec ./...

# staticcheck - advanced Go linter with security rules
staticcheck ./...

# govulncheck - official Go vulnerability scanner
govulncheck ./...

# Semgrep for security patterns
semgrep --config auto ./
```

### DAST (Dynamic Application Security Testing)

```bash
# OWASP ZAP for runtime security scanning
docker run -t owasp/zap2docker-stable zap-baseline.py \
  -t https://api.example.com \
  -r zap-report.html
```

## Code Coverage

### Target Metrics

- **Overall coverage:** 80%+
- **Critical paths:** 100% (authentication, payment, data integrity)
- **New code:** 90%+

### Commands

```bash
# Run tests with coverage
go test -coverprofile=coverage.out ./...

# View coverage report in browser
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out

# Enforce minimum coverage in CI
go test -coverprofile=coverage.out ./... && \
  go tool cover -func=coverage.out | grep total | awk '{print $3}' | \
  awk -F. '{if ($1 < 80) exit 1}'
```

## CI/CD Testing Pipeline

```yaml
# GitHub Actions example
name: Test Pipeline

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: test
          MYSQL_DATABASE: testdb
        ports:
          - 3306:3306
        options: >-
          --health-cmd "mysqladmin ping -h localhost"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Unit Tests
        run: go test -race -count=1 ./...

      - name: Integration Tests
        run: go test -race -tags=integration -count=1 ./...
        env:
          DATABASE_URL: "root:test@tcp(localhost:3306)/testdb?charset=utf8mb4&parseTime=True&loc=Local"

      - name: Coverage
        run: |
          go test -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out

      - name: Security Scan
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...

      - name: Lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest

      - name: Upload Coverage
        uses: codecov/codecov-action@v4
        with:
          files: coverage.out
```

## Testing Best Practices

1. **Arrange-Act-Assert (AAA) Pattern**
2. **Table-driven tests** - Go 惯用模式，覆盖多种输入组合
3. **t.Parallel()** - 并行执行无状态测试加速 CI
4. **t.Helper()** - 标记辅助函数，错误定位到调用方
5. **t.Cleanup()** - 注册清理函数，替代 defer
6. **TestMain()** - 全局 setup/teardown
7. **Build tags** - 用 `//go:build integration` 隔离集成测试
8. **-race flag** - 始终开启竞态检测
9. **Deterministic** - 不依赖执行顺序，不使用 time.Sleep
10. **Golden files** - 用 `testdata/` 目录存放期望输出

## Testing Checklist

- [ ] Unit tests cover 70%+ of codebase (`go test -cover`)
- [ ] Table-driven tests for all core logic
- [ ] Integration tests for all API endpoints
- [ ] Load tests configured (k6)
- [ ] Database migration tests
- [ ] Security scanning in CI/CD (gosec, govulncheck)
- [ ] Race detection enabled (`-race`)
- [ ] Code coverage reports automated
- [ ] Tests run on every PR
- [ ] Flaky tests eliminated

## Resources

- **testing:** https://pkg.go.dev/testing
- **testify:** https://github.com/stretchr/testify
- **gomock:** https://github.com/uber-go/mock
- **go-cmp:** https://github.com/google/go-cmp
- **testcontainers-go:** https://golang.testcontainers.org/
- **k6:** https://k6.io/docs/
- **gosec:** https://github.com/securego/gosec
- **govulncheck:** https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
- **golangci-lint:** https://golangci-lint.run/
