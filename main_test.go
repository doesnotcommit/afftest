package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHappy(t *testing.T) {
	const urlCount = 20
	l := log.New(os.Stdout, "test", log.LstdFlags)
	frg := newFakeResponseGenerator()
	srv := httptest.NewServer(newMultiplexer(iLimit, oLimit, errLimit, newFakeRT(frg), l))
	defer srv.Close()
	cli := http.Client{}
	buf := bytes.Buffer{}
	if err := json.NewEncoder(&buf).Encode(generateFakeURLs(urlCount)); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, io.NopCloser(&buf))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantFrg := newFakeResponseGenerator()
	wantRespBody := make([]string, urlCount)
	for i := range wantRespBody {
		wantRespBody[i] = string(wantFrg())
	}
	var body []string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(body, wantRespBody) {
		t.Errorf("want response body:\n%v\ngot:\n:%v", wantRespBody, body)
	}
}

func TestURLCountLimit(t *testing.T) {
	const urlCount = 21
	l := log.New(os.Stdout, "test", log.LstdFlags)
	frg := newFakeResponseGenerator()
	srv := httptest.NewServer(newMultiplexer(iLimit, oLimit, errLimit, newFakeRT(frg), l))
	defer srv.Close()
	cli := http.Client{}
	buf := bytes.Buffer{}
	if err := json.NewEncoder(&buf).Encode(generateFakeURLs(urlCount)); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, io.NopCloser(&buf))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	const wantStatusCode = http.StatusBadRequest
	if resp.StatusCode != wantStatusCode {
		t.Errorf("want response body:\n%d\ngot:\n:%d", wantStatusCode, resp.StatusCode)
	}
}

func generateFakeURLs(n int) []string {
	urls := make([]string, 0, n)
	for i := 0; i < n; i++ {
		urls = append(urls, fmt.Sprintf("http://url-%d", i))
	}
	return urls
}

func newFakeRT(frg func() string) rt {
	return func(r *http.Request) (*http.Response, error) {
		b := frg()
		return &http.Response{
			Body: io.NopCloser(strings.NewReader(b)),
		}, nil
	}
}

type rt func(r *http.Request) (*http.Response, error)

func (r rt) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

func newFakeResponseGenerator() func() string {
	var i int64 = 0
	return func() string {
		resp := fmt.Sprintf("response_%d", atomic.LoadInt64(&i))
		atomic.AddInt64(&i, 1)
		return resp
	}
}

func wrapWithSleep(fn func() string) func() string {
	return func() string {
		time.Sleep(5 * time.Second)
		return fn()
	}
}
