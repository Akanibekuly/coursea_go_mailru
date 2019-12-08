package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type XmlUsers struct {
	XMLName xml.Name `xml:"root"`
	Users   []MyUser `xml:"row"`
}

type MyUser struct {
	Id         int    `xml:"id" json:"id"`
	Name       string `xml:"-" json:"-"`
	FirstName  string `xml:"first_name" json:"-"`
	SecondName string `xml:"last_name" json:"-"`
	Age        int    `xml:"age" json:"age"`
	About      string `xml:"about" json:"about"`
	Gender     string `xml:"gender" json:"gender"`
}

func (u *MyUser) getFullName() string {
	return u.FirstName + " " + u.SecondName
}

func (u *MyUser) MarshalJSON() ([]byte, error) {
	type Copy MyUser

	return json.Marshal(&struct {
		Name string `json:"name"`
		*Copy
	}{
		Name: u.getFullName(),
		Copy: (*Copy)(u),
	})
}

// ------------
// implement SearchServer
// ------------
const testToken string = "12345"

type SearchServer struct {
	pathToFile string
}

func (ss *SearchServer) getUsers(params SearchRequest) ([]MyUser, error) {
	rawUsers, err := getUsersFromFile(ss.pathToFile)
	if err != nil {
		return nil, err
	}

	var resultUsers []MyUser

	if params.Query != "" {
		for _, user := range rawUsers {
			nameContainsQuery := strings.Contains(user.getFullName(), params.Query)
			aboutContainsQuery := strings.Contains(user.About, params.Query)

			if nameContainsQuery || aboutContainsQuery {
				resultUsers = append(resultUsers, user)
			}
		}
	} else {
		resultUsers = rawUsers
	}

	if params.OrderBy != 0 && params.OrderField != "" {
		sortUsers(resultUsers, params.OrderField, params.OrderBy)
	}

	if params.Offset+params.Limit > len(resultUsers) {
		return resultUsers[params.Offset:], nil
	}

	return resultUsers[params.Offset:params.Limit], nil
}

func getUsersFromFile(pathToFile string) ([]MyUser, error) {
	file, err := os.Open(pathToFile)
	if err != nil {
		return nil, errors.New("Invalid resource path")
	}

	defer file.Close()

	var usersList XmlUsers
	if err := xml.NewDecoder(file).Decode(&usersList); err != nil {
		return nil, errors.New("Error decoding file")
	}

	return usersList.Users, nil
}

func sortUsers(users []MyUser, orderField string, orderBy int) {
	sort.Slice(users, func(i, j int) bool {
		// a little bit of duplicating is better than complicating
		// and using reflection for example
		if orderField == "Id" {
			if orderBy == -1 {
				return users[i].Id > users[j].Id
			} else {
				return users[i].Id < users[j].Id
			}
		} else if orderField == "Age" {
			if orderBy == -1 {
				return users[i].Age > users[j].Age
			} else {
				return users[i].Age < users[j].Age
			}
		} else if orderField == "Name" {
			if orderBy == -1 {
				return users[i].getFullName() > users[j].getFullName()
			} else {
				return users[i].getFullName() < users[j].getFullName()
			}
		}

		// fallback
		return users[i].Id > users[j].Id
	})
}

// ------------
// HTTP Server handler
// ------------
func SearchServerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token := r.Header.Get("AccessToken")
	if token == "" || token != testToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	searchRequest, err := getValidInput(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		if err.Error() == "ErrorBadOrderField" {
			io.WriteString(w, fmt.Sprintf(`{"StatusCode": 400, "Error": "ErrorBadOrderField"}`))
		} else {
			io.WriteString(w, fmt.Sprintf(`{"StatusCode": 400, "OrderField": "%s"}`, err.Error()))
		}

		return
	}

	searchServer := SearchServer{"./dataset.xml"}

	users, err := searchServer.getUsers(searchRequest)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, fmt.Sprintf(`{"StatusCode": 500, "error": "%s"}`, err.Error()))
		return
	}

	usersJson, err := json.Marshal(users)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, fmt.Sprintf(`{"StatusCode": 500, "error": "Invalid data for json encoding"}`))
		return
	}

	io.WriteString(w, string(usersJson))
}

func getValidInput(r *http.Request) (SearchRequest, error) {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))

	if err != nil {
		return SearchRequest{}, errors.New("limit")
	}

	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))

	if err != nil {
		return SearchRequest{}, errors.New("offset")
	}

	orderBy, err := strconv.Atoi(r.URL.Query().Get("order_by"))

	if err != nil {
		return SearchRequest{}, errors.New("order_by")

	}

	orderField := r.URL.Query().Get("order_field")
	if orderField == "" {
		return SearchRequest{}, errors.New("ErrorBadOrderField")
	}

	query := r.URL.Query().Get("query")

	return SearchRequest{
		limit, offset, query, orderField, orderBy,
	}, nil
}

// ------------
// tests
// ------------
func TestRequestLimitLessThanZeroFails(t *testing.T) {
	var searchClient SearchClient
	searchRequest := SearchRequest{
		Limit: -5,
	}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error is nil for Limit < 0")
	}

	if err.Error() != "limit must be > 0" {
		t.Error("Invalid error text")
	}
}

func TestRequestOffsetLessThanZeroFails(t *testing.T) {
	var searchClient SearchClient
	searchRequest := SearchRequest{
		Offset: -5,
	}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error is nil for Offset < 0")
	}

	if err.Error() != "offset must be > 0" {
		t.Error("Invalid error text")
	}
}

func TestNoTokenFails(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(SearchServerHandler))
	defer searchService.Close()
	searchClient := &SearchClient{"Wrong token", searchService.URL}

	searchRequest := SearchRequest{}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error is nil for invalid token")
	}

	if err.Error() != "Bad AccessToken" {
		t.Error("Invalid error text")
	}
}

func TestLongServerResponseFails(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		return
	}))

	defer searchService.Close()
	searchClient := &SearchClient{testToken, searchService.URL}

	searchRequest := SearchRequest{}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Timeout reached but no error")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Error("Invalid error text")
	}
}

func TestEmptyUrlFails(t *testing.T) {
	searchClient := &SearchClient{testToken, ""}
	searchRequest := SearchRequest{}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Nil url but no error")
	}

	if !strings.Contains(err.Error(), "unknown error") {
		t.Error("Invalid error text")
	}
}

func TestServer500Fails(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}))

	defer searchService.Close()
	searchClient := &SearchClient{testToken, searchService.URL}

	searchRequest := SearchRequest{}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error must be not nil")
	}

	if err.Error() != "SearchServer fatal error" {
		t.Error("Invalid error text")
	}
}

func TestOrderFieldValidationErrorsFail(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf(`{"StatusCode": 400, "Error": "ErrorBadOrderField"}`))
		return
	}))

	defer searchService.Close()

	searchClient := &SearchClient{testToken, searchService.URL}
	searchRequest := SearchRequest{
		OrderField: "test",
	}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error must be not nil")
	}

	if err.Error() != "OrderFeld test invalid" {
		t.Error("Invalid error text")
	}
}

func TestOrderFieldValidationWrongJsonFail(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf(`Error`))
		return
	}))

	defer searchService.Close()

	searchClient := &SearchClient{testToken, searchService.URL}
	searchRequest := SearchRequest{}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error must be not nil")
	}

	if !strings.Contains(err.Error(), "cant unpack error json") {
		t.Error("Invalid error text")
	}
}

func TestValidationErrorsFail(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf(`{"StatusCode": 400, "OrderField": "Limit"}`))
		return
	}))

	defer searchService.Close()

	searchClient := &SearchClient{testToken, searchService.URL}
	searchRequest := SearchRequest{
		OrderField: "test",
	}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error must be not nil")
	}

	if !strings.Contains(err.Error(), "unknown bad request error") {
		t.Error("Invalid error text")
	}
}

func TestCorrectRequestWorks(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(SearchServerHandler))
	defer searchService.Close()
	searchClient := &SearchClient{testToken, searchService.URL}

	searchRequest := SearchRequest{
		Limit:      2,
		Offset:     0,
		OrderField: "Id",
		OrderBy:    -1,
	}

	result, err := searchClient.FindUsers(searchRequest)

	if err != nil {
		t.Error("Error must be nil")
	}

	if !result.NextPage {
		t.Error("NextPage is not valid")
	}

	if len(result.Users) != 2 {
		t.Error("Wrong users amount")
	}
}

func TestCorrectMaximumLimitWorks(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(SearchServerHandler))
	defer searchService.Close()
	searchClient := &SearchClient{testToken, searchService.URL}

	searchRequest := SearchRequest{
		Limit:      500,
		Offset:     0,
		OrderField: "Id",
		OrderBy:    -1,
	}

	result, err := searchClient.FindUsers(searchRequest)

	if err != nil {
		t.Error("Error must be nil")
	}

	if len(result.Users) != 25 {
		t.Error("Wrong users amount")
	}
}

func TestQueryWorks(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(SearchServerHandler))
	defer searchService.Close()
	searchClient := &SearchClient{testToken, searchService.URL}
	query := "Sit commodo consectetur"

	searchRequest := SearchRequest{
		Limit:      500,
		Offset:     0,
		OrderField: "Id",
		OrderBy:    -1,
		Query:      query,
	}

	result, err := searchClient.FindUsers(searchRequest)

	if err != nil {
		t.Error("Error must be nil")
	}

	for _, user := range result.Users {
		nameContainsQuery := strings.Contains(user.Name, query)
		aboutContainsQuery := strings.Contains(user.About, query)

		if !(nameContainsQuery || aboutContainsQuery) {
			t.Error("Wrong result")
		}
	}
}

func TestInvalidJsonErrorFail(t *testing.T) {
	searchService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "hello world")
		return
	}))

	defer searchService.Close()

	searchClient := &SearchClient{testToken, searchService.URL}
	searchRequest := SearchRequest{}

	_, err := searchClient.FindUsers(searchRequest)

	if err == nil {
		t.Error("Error must be not nil")
	}

	if !strings.Contains(err.Error(), "cant unpack result json") {
		t.Error("Invalid error text")
	}
}
