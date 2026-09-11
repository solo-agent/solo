package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Seed the relationship before migration 34: its old global scope is real
// historical behavior, without disabling or mocking database constraints.
func TestLegacyCompoundingMigrationPostgres(t *testing.T) {
	admin := taskSubmitTestPool(t)
	ctx := context.Background()
	name := "solo_legacy_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	keep := os.Getenv("SOLO_LEGACY_E2E_DB")
	if keep != "" {
		if !strings.HasPrefix(keep, "solo_compounding_legacy_e2e_") {
			t.Fatal("unsafe E2E database name")
		}
		name = keep
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if keep == "" {
			if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
				t.Error(err)
			}
		}
	})
	u, err := url.Parse(admin.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec(`CREATE TABLE schema_migrations(version text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())`)
	files, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrate := func(lo, hi int) {
		t.Helper()
		for _, path := range files {
			v, _ := strconv.Atoi(filepath.Base(path)[:6])
			if v < lo || v > hi {
				continue
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			exec(string(body))
			exec(`INSERT INTO schema_migrations(version) VALUES($1) ON CONFLICT DO NOTHING`, strings.TrimSuffix(filepath.Base(path), ".up.sql"))
		}
	}
	migrate(1, 33)
	owner, a, b, ch, ch2, source := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	hash, err := bcrypt.GenerateFromPassword([]byte("LegacyCompat-2026!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO users(id,email,display_name,password_hash) VALUES($1,'legacy-compat@solo.local','历史兼容验证',$2)`, owner, string(hash))
	for i, id := range []string{ch, ch2} {
		exec(`INSERT INTO channels(id,name,created_by) VALUES($1,$2,$3)`, id, fmt.Sprintf("legacy-compat-%d", i), owner)
		exec(`INSERT INTO channel_members(channel_id,member_type,member_id,role) VALUES($1,'user',$2,'owner')`, id, owner)
	}
	for i, id := range []string{a, b} {
		exec(`INSERT INTO agents(id,name,owner_id,model_provider,model_name) VALUES($1,$2,$3,'claude','sonnet')`, id, fmt.Sprintf("LegacyMember%d", i), owner)
	}
	for _, id := range []string{ch, ch2} {
		for _, agent := range []string{a, b} {
			exec(`INSERT INTO channel_members(channel_id,member_type,member_id) VALUES($1,'agent',$2)`, id, agent)
		}
	}
	exec(`INSERT INTO agent_relationships(from_agent_id,to_agent_id,rel_type,instruction) VALUES($1,$2,'assigns_to','preserve legacy coordination')`, a, b)
	migrate(34, 63)
	exec(`UPDATE users SET email_verified_at=now(),onboarding_completed_at=now() WHERE id=$1`, owner)
	exec(`INSERT INTO messages(id,channel_id,sender_type,sender_id,content) VALUES($1,$2,'user',$3,'历史重复任务来源')`, source, ch, owner)
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for i, id := range ids {
		exec(`INSERT INTO tasks(id,task_number,channel_id,creator_id,title,message_id) VALUES($1,$2,$3,$4,$5,$6)`, id, i+2, ch, owner, fmt.Sprintf("历史任务 %d", i+2), source)
	}
	migrate(64, 78)
	if count(`SELECT count(*) FROM tasks WHERE message_id=$1 AND legacy_message_source`, source) != 3 {
		t.Fatal("lost historical links")
	}
	var storedHash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, owner).Scan(&storedHash); err != nil || storedHash != string(hash) {
		t.Fatal("password changed")
	}
	svc := NewTaskService(pool)
	for i, id := range ids {
		for _, ref := range []string{id, strconv.Itoa(i + 2)} {
			task, err := svc.GetTask(ctx, ch, ref, owner)
			if err != nil || task.ID != id || task.MessageID != source {
				t.Fatalf("legacy lookup %s: %v", ref, err)
			}
		}
	}
	if _, err := svc.GetTask(ctx, ch, source, owner); !errors.Is(err, ErrTaskReferenceAmbiguous) {
		t.Fatalf("guessed GetTask: %v", err)
	}
	if _, err := svc.ConvertMessageToTask(ctx, ch, source, owner); !errors.Is(err, ErrTaskReferenceAmbiguous) {
		t.Fatalf("guessed conversion: %v", err)
	}
	if _, _, err := svc.ClaimMessageTask(ctx, ch, source, owner, nil, nil); !errors.Is(err, ErrTaskReferenceAmbiguous) {
		t.Fatalf("guessed claim: %v", err)
	}
	if _, err := svc.CreateTask(ctx, ch, owner, TaskCreateRequest{Title: "duplicate", MessageID: source}); !errors.Is(err, ErrTaskSourceExists) {
		t.Fatalf("created duplicate: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO tasks(task_number,channel_id,creator_id,title,message_id,legacy_message_source) VALUES(99,$1,$2,'bypass',$3,true)`, ch, owner, source)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("legacy flag bypass: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE tasks SET message_id=NULL WHERE id=$1`, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	var legacy bool
	if err = tx.QueryRow(ctx, `SELECT legacy_message_source FROM tasks WHERE id=$1`, ids[0]).Scan(&legacy); err != nil || legacy {
		t.Fatal("source change retained exception")
	}
	_ = tx.Rollback(ctx)
	if count(`SELECT count(*) FROM agent_relationships WHERE from_agent_id=$1 AND channel_id IS NULL`, a) != 1 {
		t.Fatal("relationship was reassigned")
	}
	if count(`SELECT count(*) FROM agent_relationship_scopes WHERE from_agent_id=$1`, a) != 2 {
		t.Fatal("legacy membership projection missing")
	}
	doc, err := NewRelationshipsMDGenerator(pool, "").RenderForAgent(ctx, a, ch)
	if err != nil || !strings.Contains(doc, "preserve legacy coordination") {
		t.Fatal("runtime lost legacy relationship", err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, edges, err := loadThinkingTeam(ctx, tx, ch)
	if err != nil || len(edges) != 1 {
		t.Fatal("Thinking lost legacy relationship", err)
	}
	_, err = tx.Exec(ctx, `DELETE FROM channel_members WHERE channel_id=$1 AND member_id=$2`, ch, b)
	if err != nil {
		t.Fatal(err)
	}
	var projected int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM agent_relationship_scopes WHERE from_agent_id=$1 AND channel_id=$2`, a, ch).Scan(&projected); err != nil || projected != 0 {
		t.Fatal("revoked membership still projects")
	}
	_ = tx.Rollback(ctx)
	if _, err = pool.Exec(ctx, `INSERT INTO agent_relationships(from_agent_id,to_agent_id,rel_type) VALUES($1,$2,'collaborates_with')`, a, b); err == nil {
		t.Fatal("new unscoped relationship accepted")
	}
	// Native uniqueness still serializes direct database writers, without service locks.
	fresh := uuid.NewString()
	exec(`INSERT INTO messages(id,channel_id,sender_type,sender_id,content) VALUES($1,$2,'user',$3,'new unique task')`, fresh, ch, owner)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := pool.Exec(ctx, `INSERT INTO tasks(task_number,channel_id,creator_id,title,message_id,legacy_message_source) VALUES($1,$2,$3,'new',$4,true)`, 100+i, ch, owner, fresh)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
			continue
		}
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatal(err)
		}
	}
	if wins != 1 || count(`SELECT count(*) FROM tasks WHERE message_id=$1 AND NOT legacy_message_source`, fresh) != 1 {
		t.Fatal("new source uniqueness broken")
	}
	if keep != "" {
		t.Log("E2E database prepared:", name)
		return
	}
	// Full feature rollback/re-upgrade keeps the original duplicate links and relation.
	for i := len(files) - 1; i >= 0; i-- {
		v, _ := strconv.Atoi(filepath.Base(files[i])[:6])
		if v < 64 {
			break
		}
		body, err := os.ReadFile(strings.TrimSuffix(files[i], ".up.sql") + ".down.sql")
		if err != nil {
			t.Fatal(err)
		}
		exec(string(body))
		exec(`DELETE FROM schema_migrations WHERE version=$1`, strings.TrimSuffix(filepath.Base(files[i]), ".up.sql"))
	}
	if count(`SELECT count(*) FROM tasks WHERE message_id=$1`, source) != 3 || count(`SELECT count(*) FROM agent_relationships WHERE from_agent_id=$1`, a) != 1 {
		t.Fatal("rollback lost legacy rows")
	}
	migrate(64, 78)
}
