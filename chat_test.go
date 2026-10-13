package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestServer поднимает тестовый сервер с hub'ом и возвращает
// его URL в формате ws://... для подключения клиентов.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	hub := newHub()
	go hub.run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	}))

	t.Cleanup(srv.Close)
	return srv
}

// wsURL превращает http URL тестового сервера в ws URL.
func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// dialAndJoin подключается и отправляет ник как первое сообщение.
func dialAndJoin(t *testing.T, url, name string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(name)); err != nil {
		t.Fatalf("send name failed: %v", err)
	}

	return conn
}

// readMessageWithTimeout читает одно сообщение с таймаутом.
// Если пришло несколько склеенных строк (батчинг в writePump),
// возвращает первую строку.
func readMessageWithTimeout(t *testing.T, conn *websocket.Conn, timeout time.Duration) string {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	s := string(msg)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func TestJoinNotification(t *testing.T) {
	srv := newTestServer(t)

	// Первый клиент подключается — он увидит сообщение о втором.
	ivan := dialAndJoin(t, wsURL(srv), "Иван")
	defer ivan.Close()

	time.Sleep(50 * time.Millisecond)

	// Второй клиент подключается.
	petr := dialAndJoin(t, wsURL(srv), "Пётр")
	defer petr.Close()

	// Иван должен получить "*** Пётр вошёл в чат ***".
	msg := readMessageWithTimeout(t, ivan, 2*time.Second)

	if !strings.Contains(msg, "Пётр") || !strings.Contains(msg, "вошёл") {
		t.Errorf("unexpected join message: %q", msg)
	}
}

func TestBroadcastWithName(t *testing.T) {
	srv := newTestServer(t)

	ivan := dialAndJoin(t, wsURL(srv), "Иван")
	defer ivan.Close()

	time.Sleep(50 * time.Millisecond)

	petr := dialAndJoin(t, wsURL(srv), "Пётр")
	defer petr.Close()

	// Пропускаем служебное сообщение о входе Петра у Ивана.
	readMessageWithTimeout(t, ivan, 2*time.Second)

	// Иван пишет сообщение.
	if err := ivan.WriteMessage(websocket.TextMessage, []byte("привет")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Пётр должен получить "Иван: привет".
	got := readMessageWithTimeout(t, petr, 2*time.Second)
	want := "Иван: привет"

	if got != want {
		t.Errorf("broadcast = %q, want %q", got, want)
	}
}

func TestSenderReceivesOwnMessage(t *testing.T) {
	srv := newTestServer(t)

	ivan := dialAndJoin(t, wsURL(srv), "Иван")
	defer ivan.Close()

	time.Sleep(50 * time.Millisecond)

	if err := ivan.WriteMessage(websocket.TextMessage, []byte("эхо")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	got := readMessageWithTimeout(t, ivan, 2*time.Second)
	want := "Иван: эхо"

	if got != want {
		t.Errorf("self broadcast = %q, want %q", got, want)
	}
}

func TestLeaveNotification(t *testing.T) {
	srv := newTestServer(t)

	ivan := dialAndJoin(t, wsURL(srv), "Иван")
	defer ivan.Close()

	time.Sleep(50 * time.Millisecond)

	petr := dialAndJoin(t, wsURL(srv), "Пётр")

	// Пропускаем "*** Пётр вошёл в чат ***".
	readMessageWithTimeout(t, ivan, 2*time.Second)

	// Пётр отключается.
	petr.Close()

	// Иван должен получить "*** Пётр покинул чат ***".
	msg := readMessageWithTimeout(t, ivan, 2*time.Second)

	if !strings.Contains(msg, "Пётр") || !strings.Contains(msg, "покинул") {
		t.Errorf("unexpected leave message: %q", msg)
	}
}

func TestNameSanitizedOnServer(t *testing.T) {
	srv := newTestServer(t)

	// Сначала подключается наблюдатель — он увидит вход "грязного".
	observer := dialAndJoin(t, wsURL(srv), "Наблюдатель")
	defer observer.Close()

	time.Sleep(50 * time.Millisecond)

	// Теперь подключается клиент с "грязным" ником.
	// Наблюдатель должен получить уже очищенный вариант.
	dirty := dialAndJoin(t, wsURL(srv), "  Мусор\n  ")
	defer dirty.Close()

	msg := readMessageWithTimeout(t, observer, 2*time.Second)

	if !strings.Contains(msg, "Мусор") {
		t.Errorf("expected sanitized name in %q", msg)
	}
	if strings.Contains(msg, "\n") {
		t.Errorf("newline leaked into broadcast: %q", msg)
	}
}
