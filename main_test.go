package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoveDuplicates(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "preserves first-seen order",
			in:   []string{"b", "a", "b", "c", "a"},
			want: []string{"b", "a", "c"},
		},
		{
			name: "trims whitespace and drops empty",
			in:   []string{"a", "", " b ", "a", "   ", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "empty input",
			in:   []string{},
			want: []string{},
		},
		{
			name: "all empty/blank",
			in:   []string{"", "  ", "\t"},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeDuplicates(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeDuplicates(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecodeBody(t *testing.T) {
	t.Run("detail object decodes", func(t *testing.T) {
		raw := []byte(`{"code":200,"body":{"name":"@candies/extended_text","version":"1.0.0","downloads":42}}`)
		got, ok, err := decodeBody[PackageBaseInfo](raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if got.Name != "@candies/extended_text" || got.Version != "1.0.0" || got.Downloads != 42 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("non-existent package (body is success string) degrades", func(t *testing.T) {
		// OHPM 对不存在的 package 仍返回 200，但 body 为字符串 "success"。
		raw := []byte(`{"code":200,"body":"success"}`)
		got, ok, err := decodeBody[PackageBaseInfo](raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("ok = true, want false for string body")
		}
		if got.Name != "" {
			t.Errorf("got.Name = %q, want empty", got.Name)
		}
	})

	t.Run("null body decodes to zero value (ok=true)", func(t *testing.T) {
		raw := []byte(`{"code":200,"body":null}`)
		got, ok, err := decodeBody[PackageBaseInfo](raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// null 可解析为零值对象；调用方再按 Name 为空降级。
		if !ok {
			t.Error("ok = false, want true for null body")
		}
		if got.Name != "" {
			t.Errorf("got.Name = %q, want empty", got.Name)
		}
	})

	t.Run("empty object body has empty name", func(t *testing.T) {
		raw := []byte(`{"code":200,"body":{}}`)
		got, ok, err := decodeBody[PackageBaseInfo](raw)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if got.Name != "" {
			t.Errorf("got.Name = %q, want empty", got.Name)
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		raw := []byte(`not json`)
		_, ok, err := decodeBody[PackageBaseInfo](raw)
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
		if ok {
			t.Error("ok = true, want false on error")
		}
	})

	t.Run("publisher rows decode", func(t *testing.T) {
		raw := []byte(`{"code":200,"body":{"rows":[{"name":"a"},{"name":"b"}]}}`)
		got, ok, err := decodeBody[PublisherInfo](raw)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if len(got.Rows) != 2 || got.Rows[0].Name != "a" || got.Rows[1].Name != "b" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("publisher end-of-pages (success string) degrades", func(t *testing.T) {
		raw := []byte(`{"code":200,"body":"success"}`)
		got, ok, err := decodeBody[PublisherInfo](raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("ok = true, want false for string body")
		}
		if len(got.Rows) != 0 {
			t.Errorf("got %+v, want no rows", got)
		}
	})
}

func TestFormatGithubInfo(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantUser string
		wantRepo string
	}{
		{"plain https url", "https://github.com/AmosHuKe/ohpm-dashboard", "AmosHuKe", "ohpm-dashboard"},
		{"with .git suffix", "https://github.com/AmosHuKe/ohpm-dashboard.git", "AmosHuKe", "ohpm-dashboard"},
		{"with fragment", "https://github.com/AmosHuKe/ohpm-dashboard#readme", "AmosHuKe", "ohpm-dashboard"},
		{"with query string", "https://github.com/AmosHuKe/ohpm-dashboard?tab=readme", "AmosHuKe", "ohpm-dashboard"},
		{"with sub path", "https://github.com/AmosHuKe/ohpm-dashboard/tree/main", "AmosHuKe", "ohpm-dashboard"},
		{"non-github url", "https://ohpm.openharmony.cn/#/cn/detail/@candies/extended_text", "", ""},
		{"empty", "", "", ""},
		{"lookalike domain is not github", "https://githubXcom/evil/repo", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, repo := formatGithubInfo(tt.in)
			if user != tt.wantUser || repo != tt.wantRepo {
				t.Errorf("formatGithubInfo(%q) = (%q, %q), want (%q, %q)", tt.in, user, repo, tt.wantUser, tt.wantRepo)
			}
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1200, "1.2k"},
		{1234, "1.23k"},
		{999999, "1000k"},
		{1000000, "1M"},
		{2500000, "2.5M"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatNumber(tt.in); got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a\nb", "a b"},
		{"a|b", "a丨b"},
		{"line1\nline2|col", "line1 line2丨col"},
		{"plain", "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := formatString(tt.in); got != tt.want {
				t.Errorf("formatString(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSortPackageInfo(t *testing.T) {
	names := func(list []PackageInfo) []string {
		out := make([]string, len(list))
		for i, p := range list {
			out[i] = p.Name
		}
		return out
	}

	t.Run("by name asc", func(t *testing.T) {
		list := []PackageInfo{{Name: "c"}, {Name: "a"}, {Name: "b"}}
		sortPackageInfo(list, "name", "asc")
		if got := names(list); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("by name desc", func(t *testing.T) {
		list := []PackageInfo{{Name: "a"}, {Name: "c"}, {Name: "b"}}
		sortPackageInfo(list, "name", "desc")
		if got := names(list); !reflect.DeepEqual(got, []string{"c", "b", "a"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("by githubStars asc", func(t *testing.T) {
		list := []PackageInfo{
			{Name: "a", GithubBaseInfo: GithubBaseInfo{StargazersCount: 30}},
			{Name: "b", GithubBaseInfo: GithubBaseInfo{StargazersCount: 10}},
			{Name: "c", GithubBaseInfo: GithubBaseInfo{StargazersCount: 20}},
		}
		sortPackageInfo(list, "githubStars", "asc")
		if got := names(list); !reflect.DeepEqual(got, []string{"b", "c", "a"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("by ohpmDownloads desc", func(t *testing.T) {
		list := []PackageInfo{
			{Name: "a", Downloads: 100},
			{Name: "b", Downloads: 300},
			{Name: "c", Downloads: 200},
		}
		sortPackageInfo(list, "ohpmDownloads", "desc")
		if got := names(list); !reflect.DeepEqual(got, []string{"b", "c", "a"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("by ohpmLikes asc", func(t *testing.T) {
		list := []PackageInfo{
			{Name: "a", Likes: 30},
			{Name: "b", Likes: 10},
			{Name: "c", Likes: 20},
		}
		sortPackageInfo(list, "ohpmLikes", "asc")
		if got := names(list); !reflect.DeepEqual(got, []string{"b", "c", "a"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("by publishTime desc means newest first", func(t *testing.T) {
		list := []PackageInfo{
			{Name: "old", PublishTime: 100},
			{Name: "new", PublishTime: 300},
			{Name: "mid", PublishTime: 200},
		}
		sortPackageInfo(list, "publishTime", "desc")
		if got := names(list); !reflect.DeepEqual(got, []string{"old", "mid", "new"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("stable for equal values", func(t *testing.T) {
		// All stars equal -> input order must be preserved (deterministic output).
		list := []PackageInfo{
			{Name: "x", GithubBaseInfo: GithubBaseInfo{StargazersCount: 5}},
			{Name: "y", GithubBaseInfo: GithubBaseInfo{StargazersCount: 5}},
			{Name: "z", GithubBaseInfo: GithubBaseInfo{StargazersCount: 5}},
		}
		sortPackageInfo(list, "githubStars", "asc")
		if got := names(list); !reflect.DeepEqual(got, []string{"x", "y", "z"}) {
			t.Errorf("expected stable order x,y,z, got %v", got)
		}
	})
}

func TestHTTPGetWithRetry(t *testing.T) {
	client := newHTTPClient()

	t.Run("success on first attempt", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		body, status, err := httpGetWithRetry(context.Background(), client, srv.URL, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != http.StatusOK {
			t.Errorf("status = %d, want 200", status)
		}
		if string(body) != `{"ok":true}` {
			t.Errorf("body = %q", body)
		}
		if n := hits.Load(); n != 1 {
			t.Errorf("hits = %d, want 1", n)
		}
	})

	t.Run("retries on 503 then succeeds", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) < int32(maxAttempts) {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("recovered"))
		}))
		defer srv.Close()

		body, status, err := httpGetWithRetry(context.Background(), client, srv.URL, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != http.StatusOK || string(body) != "recovered" {
			t.Errorf("status=%d body=%q, want 200/recovered", status, body)
		}
		if n := hits.Load(); n != int32(maxAttempts) {
			t.Errorf("hits = %d, want %d", n, maxAttempts)
		}
	})

	t.Run("retries on 429 then succeeds", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) < int32(maxAttempts) {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}))
		defer srv.Close()

		_, status, err := httpGetWithRetry(context.Background(), client, srv.URL, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != http.StatusOK {
			t.Errorf("status = %d, want 200", status)
		}
		if n := hits.Load(); n != int32(maxAttempts) {
			t.Errorf("hits = %d, want %d", n, maxAttempts)
		}
	})

	t.Run("404 is not retried", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		_, status, err := httpGetWithRetry(context.Background(), client, srv.URL, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404", status)
		}
		if n := hits.Load(); n != 1 {
			t.Errorf("hits = %d, want 1 (404 must not retry)", n)
		}
	})

	t.Run("exhausts retries on persistent 503", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		_, _, err := httpGetWithRetry(context.Background(), client, srv.URL, nil)
		if err == nil {
			t.Fatal("expected error after exhausting retries")
		}
		if n := hits.Load(); n != int32(maxAttempts) {
			t.Errorf("hits = %d, want %d", n, maxAttempts)
		}
	})

	t.Run("respects cancelled context", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消

		_, _, err := httpGetWithRetry(ctx, client, srv.URL, nil)
		if err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})

	t.Run("sends provided headers", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		_, _, err := httpGetWithRetry(context.Background(), client, srv.URL, map[string]string{"Authorization": "bearer xyz"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotAuth != "bearer xyz" {
			t.Errorf("Authorization header = %q, want %q", gotAuth, "bearer xyz")
		}
	})
}

func TestConcurrentMap(t *testing.T) {
	t.Run("preserves order despite out-of-order completion", func(t *testing.T) {
		items := []int{0, 1, 2, 3, 4, 5, 6, 7}
		got, err := concurrentMap(context.Background(), items, 4, func(ctx context.Context, n int) (int, error) {
			// 越靠前的元素睡得越久 -> 越晚完成，制造乱序完成
			time.Sleep(time.Duration(10*(len(items)-n)) * time.Millisecond)
			return n * 10, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []int{0, 10, 20, 30, 40, 50, 60, 70}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("returns the first error", func(t *testing.T) {
		sentinel := errors.New("boom")
		items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		_, err := concurrentMap(context.Background(), items, 2, func(ctx context.Context, n int) (int, error) {
			if n == 1 {
				return 0, sentinel
			}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return n, nil
			}
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v, want sentinel %v", err, sentinel)
		}
	})

	t.Run("respects concurrency limit", func(t *testing.T) {
		const limit = 3
		var current, max atomic.Int32
		items := make([]int, 30)
		_, err := concurrentMap(context.Background(), items, limit, func(ctx context.Context, n int) (int, error) {
			c := current.Add(1)
			for { // 记录观察到的并发峰值
				m := max.Load()
				if c <= m || max.CompareAndSwap(m, c) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
			return 0, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m := max.Load(); m > limit {
			t.Errorf("max observed concurrency = %d, want <= %d", m, limit)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		got, err := concurrentMap(context.Background(), []int{}, 4, func(ctx context.Context, n int) (int, error) {
			return n, nil
		})
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, err %v", got, err)
		}
	})
}
