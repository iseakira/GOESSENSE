package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFindEntries(t *testing.T){
	ts := httptest.NewServer(http.HandleFunc(func(w http.ResponseWriter,r *http.Request){
		//処理
	}))
	defer ts.Close()
}

