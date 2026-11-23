package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/migrations"
	"github.com/logoes0/peeriodic.git/models"
	"github.com/logoes0/peeriodic.git/ot"
	"github.com/logoes0/peeriodic.git/routers"
	"github.com/logoes0/peeriodic.git/services"
)

func testDatabase(t *testing.T) *services.DatabaseService {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	parsed, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	ssl := parsed.Query().Get("sslmode")
	if ssl == "" {
		ssl = "disable"
	}
	cfg := &config.Config{Database: config.DatabaseConfig{User: parsed.User.Username(), Password: password,
		Host: parsed.Hostname(), Port: port, Name: strings.TrimPrefix(parsed.Path, "/"), SSLMode: ssl}}
	db, err := services.NewDatabaseService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := migrations.NewRunner(db.GetDB()).ApplyAll(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func api(t *testing.T, base, token, method, path, body string, want int) json.RawMessage {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}
	if resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status=%d want=%d error=%s", method, path, resp.StatusCode, want, payload.Error)
	}
	return payload.Data
}

func readMessage(t *testing.T, conn *websocket.Conn, kind string) models.Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 20; i++ {
		var msg models.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("waiting for %s: %v", kind, err)
		}
		if msg.Type == kind {
			return msg
		}
		if msg.Type == models.MsgTypeError {
			t.Fatalf("unexpected server error: %s", msg.Data)
		}
	}
	t.Fatalf("never received %s", kind)
	return models.Message{}
}

func TestBackendIntegration(t *testing.T) {
	db := testDatabase(t)
	ctx := t.Context()
	cfg := &config.Config{Auth: config.AuthConfig{JWTSecret: strings.Repeat("t", 32), TokenExpiryHours: 1},
		WebSocket: config.WebSocketConfig{AllowedOrigins: []string{"http://localhost:3000"}}}
	auth := services.NewAuthService(cfg)
	ws := services.NewWebSocketService(cfg, auth)
	server := httptest.NewServer(routers.NewRouter(db, ws, auth).SetupRoutes())
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ws.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	email := uuid.NewString() + "@example.com"
	registration := api(t, server.URL, "", "POST", "/api/auth/register", `{"email":"`+email+`","password":"test-password","name":"Owner"}`, 200)
	var owner models.AuthResponse
	if err := json.Unmarshal(registration, &owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.GetDB().Exec(`DELETE FROM users WHERE uid = $1`, owner.User.UID) })
	api(t, server.URL, "", "POST", "/api/auth/register", `{"email":"`+email+`","password":"test-password"}`, 409)
	api(t, server.URL, "", "POST", "/api/auth/login", `{"email":"`+email+`","password":"test-password"}`, 200)

	memberUID := uuid.NewString()
	if _, err := db.CreateUserWithPassword(ctx, memberUID, memberUID+"@example.com", "Member", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.GetDB().Exec(`DELETE FROM users WHERE uid = $1`, memberUID) })
	memberToken, err := auth.GenerateToken(memberUID, memberUID+"@example.com", "Member")
	if err != nil {
		t.Fatal(err)
	}
	data := api(t, server.URL, owner.Token, "POST", "/api/rooms", `{"title":"Private document","uid":"`+owner.User.UID+`","email":"forged@example.com","name":"Forged"}`, 200)
	var room services.Room
	if err := json.Unmarshal(data, &room); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.GetDB().Exec(`DELETE FROM rooms WHERE id = $1`, room.ID) })
	if _, err := db.GetDB().ExecContext(ctx, `UPDATE rooms SET public_role = 'none' WHERE id = $1`, room.ID); err != nil {
		t.Fatal(err)
	}
	storedOwner, err := db.GetUserByUID(ctx, owner.User.UID)
	if err != nil || storedOwner.Email != email || storedOwner.Name != "Owner" {
		t.Fatalf("room creation changed account: %+v %v", storedOwner, err)
	}
	members, err := db.ListRoomMembers(ctx, room.ID)
	if err != nil || len(members) != 1 || members[0].Role != services.RoleOwner {
		t.Fatalf("owner membership missing: %+v %v", members, err)
	}
	api(t, server.URL, memberToken, "GET", "/api/rooms?uid="+owner.User.UID, "", 403)
	api(t, server.URL, "", "GET", "/api/rooms?uid="+owner.User.UID, "", 401)
	api(t, server.URL, memberToken, "POST", "/api/rooms", `{"uid":"`+owner.User.UID+`"}`, 403)
	api(t, server.URL, "broken", "GET", "/api/rooms/"+room.ID, "", 401)
	api(t, server.URL, memberToken, "GET", "/api/rooms/"+room.ID, "", 403)
	api(t, server.URL, owner.Token, "POST", "/api/rooms", "{} garbage", 400)
	api(t, server.URL, owner.Token, "POST", "/api/rooms", `{"title":"valid"} {}`, 400)
	api(t, server.URL, owner.Token, "POST", "/api/rooms", "null", 400)
	api(t, server.URL, owner.Token, "POST", "/api/rooms/"+room.ID+"/invites/extra", `{}`, 404)

	inviteData := api(t, server.URL, owner.Token, "POST", "/api/rooms/"+room.ID+"/invites", `{"role":"editor","expires_in_hours":1}`, 200)
	var invite models.RoomInvite
	if err := json.Unmarshal(inviteData, &invite); err != nil {
		t.Fatal(err)
	}
	api(t, server.URL, memberToken, "POST", "/api/invites/"+invite.Token+"/accept", "", 200)
	listed := api(t, server.URL, memberToken, "GET", "/api/rooms", "", 200)
	var listedRooms []models.Room
	if err := json.Unmarshal(listed, &listedRooms); err != nil {
		t.Fatal(err)
	}
	if len(listedRooms) != 1 || listedRooms[0].Role != services.RoleEditor {
		t.Fatalf("shared room missing: %s", listed)
	}
	// A weaker invite cannot downgrade an editor or the owner.
	viewerInvite, err := db.CreateRoomInvite(ctx, room.ID, uuid.NewString(), services.RoleViewer, &owner.User.UID, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, role, err := db.AcceptRoomInvite(ctx, viewerInvite.Token, memberUID)
	if err != nil || role != services.RoleEditor {
		t.Fatalf("invite downgraded editor: %s %v", role, err)
	}
	_, role, err = db.AcceptRoomInvite(ctx, viewerInvite.Token, owner.User.UID)
	if err != nil || role != services.RoleOwner {
		t.Fatalf("invite downgraded owner: %s %v", role, err)
	}
	api(t, server.URL, owner.Token, "DELETE", "/api/rooms/"+room.ID+"/members/"+owner.User.UID, "", 403)

	wsBase := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?room=" + room.ID
	for _, tc := range []struct {
		query, origin string
		status        int
	}{
		{"&uid=" + owner.User.UID, "", 403}, {"&uid=" + owner.User.UID + "&token=broken", "", 401},
		{"&token=" + owner.Token, "https://attacker.example", 403},
	} {
		headers := http.Header{}
		if tc.origin != "" {
			headers.Set("Origin", tc.origin)
		}
		conn, resp, err := websocket.DefaultDialer.Dial(wsBase+tc.query, headers)
		if conn != nil {
			conn.Close()
		}
		if err == nil || resp == nil || resp.StatusCode != tc.status {
			t.Fatalf("unexpected WS authentication result: resp=%v err=%v", resp, err)
		}
		resp.Body.Close()
	}
	dial := func(token string) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial(wsBase+"&token="+token, http.Header{"Origin": []string{"http://localhost:3000"}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		init := readMessage(t, conn, models.MsgTypeInit)
		if !services.CanRead(init.Role) {
			t.Fatalf("invalid role: %s", init.Role)
		}
		return conn
	}
	first, second := dial(owner.Token), dial(memberToken)
	send := func(conn *websocket.Conn, rev int, ops ...ot.Operation) {
		t.Helper()
		if err := conn.WriteJSON(models.Message{Type: models.MsgTypeOp, Rev: rev, Ops: ops}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func(want string, rev int) {
		t.Helper()
		stored, err := db.GetRoom(ctx, room.ID)
		if err != nil || stored.Content != want || stored.CurrentRev != rev {
			t.Fatalf("snapshot=%+v want=%q rev=%d err=%v", stored, want, rev, err)
		}
	}
	send(first, 0, ot.Operation{Type: ot.OpInsert, Text: "hello"})
	if ack := readMessage(t, first, models.MsgTypeAck); ack.Rev != 1 {
		t.Fatal(ack)
	}
	snapshot("hello", 1)
	readMessage(t, second, models.MsgTypeOp)
	send(first, 1, ot.Operation{Type: ot.OpDelete, Len: 5}, ot.Operation{Type: ot.OpInsert, Text: "world"})
	readMessage(t, first, models.MsgTypeAck)
	readMessage(t, second, models.MsgTypeOp)
	snapshot("world", 2)
	var opCount int
	if err := db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM room_operations WHERE room_id = $1 AND rev = 2`, room.ID).Scan(&opCount); err != nil || opCount != 2 {
		t.Fatalf("batch log lost operations: count=%d err=%v", opCount, err)
	}
	// An operation-log failure must roll back its snapshot update.
	err = db.SaveRoomRevision(ctx, room.ID, "should rollback", 3, strings.Repeat("x", 256), []ot.Operation{{Type: ot.OpInsert, Text: "x"}})
	if err == nil {
		t.Fatal("expected operation-log insert to fail")
	}
	snapshot("world", 2)
	if err = db.SaveRoomRevision(ctx, room.ID, "stale", 2, "test", nil); !errors.Is(err, services.ErrConflict) {
		t.Fatalf("stale snapshot accepted: %v", err)
	}
	// Force a revision conflict and verify that the socket sends resync, never ack.
	if _, err = db.GetDB().ExecContext(ctx, `UPDATE rooms SET current_rev = 3 WHERE id = $1`, room.ID); err != nil {
		t.Fatal(err)
	}
	send(first, 2, ot.Operation{Type: ot.OpInsert, Text: "lost"})
	if msg := readMessage(t, first, models.MsgTypeSync); msg.Rev != 2 || msg.Data != "world" {
		t.Fatal(msg)
	}
	if _, err = db.GetDB().ExecContext(ctx, `UPDATE rooms SET current_rev = 2 WHERE id = $1`, room.ID); err != nil {
		t.Fatal(err)
	}

	var writers sync.WaitGroup
	for i, conn := range []*websocket.Conn{first, second} {
		writers.Add(1)
		go func(conn *websocket.Conn, text string) {
			defer writers.Done()
			if err := conn.WriteJSON(models.Message{Type: models.MsgTypeOp, Rev: 2, Ops: []ot.Operation{{Type: ot.OpInsert, Text: text}}}); err != nil {
				t.Error(err)
			}
		}(conn, string(rune('A'+i)))
	}
	writers.Wait()
	for _, conn := range []*websocket.Conn{first, second} {
		seen := map[string]bool{}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for len(seen) < 2 {
			var msg models.Message
			if err := conn.ReadJSON(&msg); err != nil {
				t.Fatal(err)
			}
			if msg.Type == models.MsgTypeAck || msg.Type == models.MsgTypeOp {
				seen[msg.Type] = true
				if msg.Rev < 3 || msg.Rev > 4 {
					t.Fatal(msg)
				}
			}
		}
	}
	stored, err := db.GetRoom(ctx, room.ID)
	if err != nil || stored.CurrentRev != 4 || (stored.Content != "ABworld" && stored.Content != "BAworld") {
		t.Fatalf("concurrent edits: %+v %v", stored, err)
	}
	api(t, server.URL, owner.Token, "POST", "/api/save?room="+room.ID, `{"content":"stale"}`, 409)
	body, _ := json.Marshal(map[string]string{"content": stored.Content})
	api(t, server.URL, owner.Token, "POST", "/api/save?room="+room.ID, string(body), 200)
	send(first, 4, ot.Operation{Type: ot.OpDelete, Len: 7})
	readMessage(t, first, models.MsgTypeAck)
	readMessage(t, second, models.MsgTypeOp)
	snapshot("", 5)
	if err := first.WriteJSON(models.Message{Type: models.MsgTypeSync}); err != nil {
		t.Fatal(err)
	}
	_ = first.SetReadDeadline(time.Now().Add(5 * time.Second))
	var emptyWire map[string]json.RawMessage
	if err := first.ReadJSON(&emptyWire); err != nil {
		t.Fatal(err)
	}
	if string(emptyWire["data"]) != `""` || string(emptyWire["rev"]) != "5" {
		t.Fatalf("empty snapshot missing from wire: %s", emptyWire)
	}
	api(t, server.URL, owner.Token, "POST", "/api/save?room="+room.ID, `{"content":""}`, 200)
	api(t, server.URL, owner.Token, "POST", "/api/save?room="+room.ID, `{}`, 400)
	api(t, server.URL, owner.Token, "POST", "/api/save?room="+room.ID, `{"content":null}`, 400)

	api(t, server.URL, owner.Token, "PUT", "/api/rooms/"+room.ID+"/members/"+memberUID, `{"role":"viewer"}`, 200)
	second.Close()
	second = dial(memberToken)
	send(second, 5, ot.Operation{Type: ot.OpInsert, Text: "forbidden"})
	_ = second.SetReadDeadline(time.Now().Add(5 * time.Second))
	var denied models.Message
	if err := second.ReadJSON(&denied); err != nil || denied.Type != models.MsgTypeError {
		t.Fatalf("viewer edit: %+v %v", denied, err)
	}
	snapshot("", 5)
	api(t, server.URL, owner.Token, "DELETE", "/api/rooms/"+room.ID+"/members/"+memberUID, "", 200)
	if err := second.WriteJSON(models.Message{Type: models.MsgTypeSync}); err != nil {
		t.Fatal(err)
	}
	if err := second.ReadJSON(&denied); err == nil {
		t.Fatal("removed member can still sync")
	}
	first.Close()
	reconnected := dial(owner.Token)
	if err := reconnected.WriteJSON(models.Message{Type: models.MsgTypeSync}); err != nil {
		t.Fatal(err)
	}
	if msg := readMessage(t, reconnected, models.MsgTypeSync); msg.Rev != 5 || msg.Data != "" {
		t.Fatal(msg)
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := ws.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if len(ws.GetRoomStats()) != 0 {
		t.Fatal("shutdown retained clients")
	}
	// The old global ServeMux prevented creating more than one router in a process.
	routers.NewRouter(db, ws, auth).SetupRoutes()
}

func TestConcurrentMigrations(t *testing.T) {
	db := testDatabase(t)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if err := migrations.NewRunner(db.GetDB()).ApplyAll(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
}

func TestGuestRoomCreationAndPersistence(t *testing.T) {
	db := testDatabase(t)
	ctx := t.Context()
	cfg := &config.Config{Auth: config.AuthConfig{JWTSecret: strings.Repeat("s", 32), TokenExpiryHours: 1}}
	auth := services.NewAuthService(cfg)
	ws := services.NewWebSocketService(cfg, auth)
	server := httptest.NewServer(routers.NewRouter(db, ws, auth).SetupRoutes())
	defer server.Close()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := ws.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	roomID := uuid.NewString()
	t.Cleanup(func() { _, _ = db.GetDB().Exec(`DELETE FROM rooms WHERE id=$1`, roomID) })
	api(t, server.URL, "", "GET", "/ws?room="+roomID, "", 400)
	if _, err := db.GetRoom(ctx, roomID); !errors.Is(err, services.ErrNotFound) {
		t.Fatalf("plain HTTP created a room: %v", err)
	}
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?room=" + roomID
	conn, resp, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Origin": []string{"https://attacker.example"}})
	if conn != nil {
		conn.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatalf("foreign origin was accepted: %v", err)
	}
	resp.Body.Close()
	if _, err := db.GetRoom(ctx, roomID); !errors.Is(err, services.ErrNotFound) {
		t.Fatal("foreign origin created a room")
	}
	conn, _, err = websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	readMessage(t, conn, models.MsgTypeInit)
	if err := conn.WriteJSON(models.Message{Type: models.MsgTypeOp, Ops: []ot.Operation{{Type: ot.OpInsert, Text: "guest content"}}}); err != nil {
		t.Fatal(err)
	}
	readMessage(t, conn, models.MsgTypeAck)
	stored, err := db.GetRoom(ctx, roomID)
	if err != nil || stored.Content != "guest content" || stored.CurrentRev != 1 || stored.UserUID != nil {
		t.Fatalf("guest persistence failed: %+v %v", stored, err)
	}
}
