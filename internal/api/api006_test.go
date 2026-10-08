package api

import (
	"fmt"
	"strings"
	"testing"

	"expense-bot/internal/catalog"
	"expense-bot/internal/ledger"
)

// Tests for feature 006: per-user categories.

func catList(t *testing.T, e testEnv, auth string) (int64, []map[string]any) {
	t.Helper()
	code, b := e.do(t, "GET", "/api/categories", auth, "")
	if code != 200 {
		t.Fatalf("list: %d %v", code, b)
	}
	var items []map[string]any
	for _, it := range b["items"].([]any) {
		items = append(items, it.(map[string]any))
	}
	return num(b["version"]), items
}

func findCat(items []map[string]any, id string) map[string]any {
	for _, it := range items {
		if it["id"] == id {
			return it
		}
	}
	return nil
}

func lineOf(s map[string]any, id string) map[string]any {
	for _, c := range s["categories"].([]any) {
		if m := c.(map[string]any); m["id"] == id {
			return m
		}
	}
	return nil
}

func TestCategoriesDefaults(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	_, s := e.do(t, "POST", "/api/session", auth, "")
	cats := s["categories"].([]any)
	first := cats[0].(map[string]any)
	if len(cats) != 14 || first["id"] != "c_food" || first["name"] != "🍽 Еда/Продукты" || num(first["limit"]) != 200000 || first["isDefault"] != true {
		t.Errorf("defaults: %d, first %v", len(cats), first)
	}
	if num(s["categoriesVersion"]) != 1 {
		t.Errorf("version = %v", s["categoriesVersion"])
	}
}

func TestCategoriesAdd(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	body := `{"clientId":"` + uuidN(1) + `","name":"🐱 Кошка","limit":30000}`
	code, b := e.do(t, "POST", "/api/categories", auth, body)
	items := b["items"].([]any)
	last := items[len(items)-1].(map[string]any)
	if code != 201 || len(items) != 15 || last["name"] != "🐱 Кошка" || num(last["position"]) != 14 {
		t.Fatalf("add: %d %v", code, last)
	}
	catID := last["id"].(string)
	if catID != catalog.ClientID(uuidN(1)) {
		t.Errorf("id = %s", catID)
	}
	if code, b := e.do(t, "POST", "/api/categories", auth, body); code != 200 || len(b["items"].([]any)) != 15 {
		t.Errorf("replay: %d", code)
	}
	if code, b := e.do(t, "POST", "/api/categories", auth, `{"clientId":"`+uuidN(2)+`","name":" 🐱 кошка "}`); code != 409 || errCode(b) != "duplicate" {
		t.Errorf("duplicate: %d %v", code, b)
	}
	for _, name := range []string{"", strings.Repeat("я", 31)} {
		if code, _ := e.do(t, "POST", "/api/categories", auth, `{"clientId":"`+uuidN(3)+`","name":"`+name+`"}`); code != 400 {
			t.Errorf("name %q: %d", name, code)
		}
	}
	for i := 15; i < catalog.MaxEntries; i++ {
		e.do(t, "POST", "/api/categories", auth, fmt.Sprintf(`{"clientId":"%s","name":"n%d"}`, uuidN(100+i), i))
	}
	if code, b := e.do(t, "POST", "/api/categories", auth, `{"clientId":"`+uuidN(999)+`","name":"over"}`); code != 422 || errCode(b) != "limit_reached" {
		t.Errorf("51st: %d %v", code, b)
	}
	code, _ = e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuidN(500)+`","kind":"expense","amount":12000,"category":"`+catID+`"}`)
	if code != 201 {
		t.Fatalf("expense in new category: %d", code)
	}
	l := lineOf(summaryOf(t, e, auth, ""), catID)
	if l == nil || l["label"] != "🐱 Кошка" || num(l["limit"]) != 30000 || num(l["spent"]) != 12000 {
		t.Errorf("summary line = %v", l)
	}
}

func TestCategoriesRename(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	e.do(t, "POST", "/api/session", initData(7), "")
	e.mem.seed(uid, "a", ledger.KindExpense, "c_transport", 5000, "", monthsAgo(1))
	e.mem.seed(uid, "b", ledger.KindExpense, "c_transport", 7000, "", monthsAgo(0))

	ver, _ := catList(t, e, auth)
	code, b := e.do(t, "PATCH", "/api/categories/c_transport", auth, fmt.Sprintf(`{"version":%d,"name":"🚕 Такси и автобус"}`, ver))
	if code != 200 {
		t.Fatalf("rename: %d %v", code, b)
	}
	_, items := catList(t, e, auth)
	if c := findCat(items, "c_transport"); c["name"] != "🚕 Такси и автобус" || num(c["limit"]) != 100000 || num(c["position"]) != 2 {
		t.Errorf("after rename: %v", c)
	}
	for _, m := range []string{ledger.MonthKey(monthsAgo(1)), ledger.MonthKey(monthsAgo(0))} {
		if l := lineOf(summaryOf(t, e, auth, m), "c_transport"); l == nil || l["label"] != "🚕 Такси и автобус" {
			t.Errorf("%s line = %v", m, l)
		}
	}
	_, h := e.do(t, "GET", "/api/transactions?category=c_transport", auth, "")
	for _, it := range h["items"].([]any) {
		if x := it.(map[string]any); x["category"] != "c_transport" || num(x["version"]) != 1 {
			t.Errorf("record touched by rename: %v", x)
		}
	}
	ver2, _ := catList(t, e, auth)
	if code, b := e.do(t, "PATCH", "/api/categories/c_transport", auth, fmt.Sprintf(`{"version":%d,"name":"🍽 Еда/Продукты"}`, ver2)); code != 409 || errCode(b) != "duplicate" {
		t.Errorf("duplicate rename: %d %v", code, b)
	}
	code, b = e.do(t, "PATCH", "/api/categories/c_transport", auth, fmt.Sprintf(`{"version":%d,"name":"x"}`, ver))
	if code != 409 || errCode(b) != "conflict" || b["error"].(map[string]any)["current"].(map[string]any)["items"] == nil {
		t.Errorf("stale: %d %v", code, b)
	}
	_, other := catList(t, e, initData(7))
	if findCat(other, "c_transport")["name"] != "🚗 Транспорт" {
		t.Error("other user's category renamed")
	}
	if code, _ := e.do(t, "PATCH", "/api/categories/c_nope", auth, fmt.Sprintf(`{"version":%d,"name":"x"}`, ver2)); code != 404 {
		t.Errorf("missing: %d", code)
	}
}

func TestCategoriesHide(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	e.mem.seed(uid, "kid", ledger.KindExpense, "c_kids_study", 9000, "", monthsAgo(1))

	ver, _ := catList(t, e, auth)
	if code, _ := e.do(t, "PATCH", "/api/categories/c_kids_study", auth, fmt.Sprintf(`{"version":%d,"hidden":true}`, ver)); code != 200 {
		t.Fatalf("hide: %d", code)
	}
	if l := lineOf(summaryOf(t, e, auth, ledger.MonthKey(monthsAgo(1))), "c_kids_study"); l == nil || l["hidden"] != true || l["status"] != "none" {
		t.Errorf("hidden line with spending = %v", l)
	}
	if l := lineOf(summaryOf(t, e, auth, ""), "c_kids_study"); l != nil {
		t.Errorf("hidden line without spending = %v", l)
	}
	if _, h := e.do(t, "GET", "/api/transactions?category=c_kids_study", auth, ""); len(h["items"].([]any)) != 1 {
		t.Error("history lost hidden category record")
	}
	if code, _ := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuidN(1)+`","kind":"expense","amount":100,"category":"c_kids_study"}`); code != 400 {
		t.Errorf("new record in hidden: %d", code)
	}
	if code, b := e.do(t, "PATCH", "/api/transactions/kid", auth, `{"version":1,"requestId":"`+uuidN(2)+`","note":"школа"}`); code != 200 || b["transaction"].(map[string]any)["category"] != "c_kids_study" {
		t.Errorf("edit note in hidden: %d %v", code, b)
	}
	if code, _ := e.do(t, "PATCH", "/api/transactions/kid", auth, `{"version":2,"requestId":"`+uuidN(3)+`","category":"c_kids_study"}`); code != 400 {
		t.Errorf("change to hidden: %d", code)
	}

	// hide all but one, then the last
	ver, items := catList(t, e, auth)
	visible := []string{}
	for _, it := range items {
		if it["hidden"] == false {
			visible = append(visible, it["id"].(string))
		}
	}
	for _, id := range visible[1:] {
		code, b := e.do(t, "PATCH", "/api/categories/"+id, auth, fmt.Sprintf(`{"version":%d,"hidden":true}`, ver))
		if code != 200 {
			t.Fatalf("hide %s: %d %v", id, code, b)
		}
		ver = num(b["version"])
	}
	if code, b := e.do(t, "PATCH", "/api/categories/"+visible[0], auth, fmt.Sprintf(`{"version":%d,"hidden":true}`, ver)); code != 422 || errCode(b) != "last_visible" {
		t.Errorf("last visible: %d %v", code, b)
	}
	code, b := e.do(t, "PATCH", "/api/categories/c_kids_study", auth, fmt.Sprintf(`{"version":%d,"hidden":false}`, ver))
	if code != 200 {
		t.Fatalf("show: %d %v", code, b)
	}
	if code, _ := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuidN(4)+`","kind":"expense","amount":100,"category":"c_kids_study"}`); code != 201 {
		t.Errorf("new record after show: %d", code)
	}
}

func TestCategoriesOrder(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	ver, items := catList(t, e, auth)
	ids := make([]string, len(items))
	for i, it := range items {
		ids[len(items)-1-i] = `"` + it["id"].(string) + `"`
	}
	body := fmt.Sprintf(`{"version":%d,"ids":[%s]}`, ver, strings.Join(ids, ","))
	code, b := e.do(t, "POST", "/api/categories/order", auth, body)
	if code != 200 || b["items"].([]any)[0].(map[string]any)["id"] != "c_unknown" {
		t.Fatalf("order: %d %v", code, b)
	}
	newVer := num(b["version"])
	if code, _ := e.do(t, "POST", "/api/categories/order", auth, fmt.Sprintf(`{"version":%d,"ids":[%s]}`, newVer, strings.Join(ids[1:], ","))); code != 400 {
		t.Errorf("missing id: %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/categories/order", auth, body); code != 409 {
		t.Errorf("stale order: %d", code)
	}
}
