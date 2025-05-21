package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	e       *echo.Echo
	coll    *mongo.Collection
	testCtx context.Context
)

func TestMain(m *testing.M) {
	os.Setenv("SKIP_TEMPLATES", "true")
	os.Exit(m.Run())
}

func setup() {
	testCtx, _ = context.WithTimeout(context.Background(), 10*time.Second)
	client, err := mongo.Connect(testCtx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		panic(err)
	}

	coll, err = prepareDatabase(client, "exercise-1", "information")
	if err != nil {
		panic(err)
	}

	prepareData(client, coll)

	e = echo.New()
	e.Renderer = loadTemplates()

	e.GET("/api/books", func(c echo.Context) error {
		books := findAllBooks(coll)
		return c.JSON(http.StatusOK, books)
	})

	e.POST("/api/books", func(c echo.Context) error {
		var book BookStore
		if err := c.Bind(&book); err != nil {
			return c.String(http.StatusBadRequest, "Invalid request")
		}

		if book.ID == "" || book.BookName == "" {
			return c.String(http.StatusBadRequest, "Missing required fields: id and title")
		}

		filter := bson.M{
			"ID":          book.ID,
			"BookName":    book.BookName,
			"BookAuthor":  book.BookAuthor,
			"BookEdition": book.BookEdition,
			"BookPages":   book.BookPages,
			"BookYear":    book.BookYear,
		}

		count, err := coll.CountDocuments(context.TODO(), filter)
		if err != nil {
			return c.String(http.StatusInternalServerError, "DB error")
		}
		if count > 0 {
			return c.String(http.StatusConflict, "Duplicate entry")
		}

		_, err = coll.InsertOne(context.TODO(), book)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Insert error")
		}

		return c.NoContent(http.StatusCreated)

	})

	e.PUT("/api/books/:id", func(c echo.Context) error {
		id := c.Param("id")
		var book BookStore
		if err := c.Bind(&book); err != nil {
			return c.String(http.StatusBadRequest, "Invalid request")
		}

		filter := bson.M{"ID": id}
		update := bson.M{"$set": bson.M{
			"BookName":    book.BookName,
			"BookAuthor":  book.BookAuthor,
			"BookEdition": book.BookEdition,
			"BookPages":   book.BookPages,
			"BookYear":    book.BookYear,
		}}

		res, err := coll.UpdateOne(context.TODO(), filter, update)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Update error")
		}
		if res.MatchedCount == 0 {
			return c.String(http.StatusNotFound, "Book not found")
		}

		return c.NoContent(http.StatusOK)
	})

	e.DELETE("/api/books/:id", func(c echo.Context) error {
		id := c.Param("id")
		res, err := coll.DeleteOne(context.TODO(), bson.M{"ID": id})
		if err != nil {
			return c.String(http.StatusInternalServerError, "Delete error")
		}
		if res.DeletedCount == 0 {
			return c.String(http.StatusNotFound, "Book not found")
		}

		return c.NoContent(http.StatusOK)
	})
}

func TestBookAPI(t *testing.T) {
	setup()

	newBook := map[string]string{
		"ID":          "asd34343",
		"BookName":    "The book name",
		"BookAuthor":  "The book author",
		"BookEdition": "1st Edition",
		"BookPages":   "1000",
		"BookYear":    "1900",
	}
	body, _ := json.Marshal(newBook)
	req := httptest.NewRequest(http.MethodPost, "/api/books", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("POST /api/books expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/books expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var books []map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &books)
	if err != nil {
		t.Fatalf("Error unmarshaling GET response: %v", err)
	}
	found := false
	for _, book := range books {
		if book["title"] == "The book name" && book["id"] == "asd34343" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Newly added book not found in GET /api/books response")
	}

	update := map[string]string{
		"ID":          "asd34343",
		"BookName":    "Updated Name",
		"BookAuthor":  "Updated author",
		"BookEdition": "1st Edition",
		"BookPages":   "1000",
		"BookYear":    "1900",
	}
	body, _ = json.Marshal(update)
	req = httptest.NewRequest(http.MethodPut, "/api/books/asd34343", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("PUT /api/books/:id expected status %d, got %d", http.StatusOK, rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var booksAfterUpdate []map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &booksAfterUpdate)
	if err != nil {
		t.Fatalf("Error unmarshaling GET response after update: %v", err)
	}

	updatedFound := false
	for _, book := range booksAfterUpdate {
		if book["title"] == "Updated Name" && book["author"] == "Updated author" {
			updatedFound = true
			break
		}
	}
	if !updatedFound {
		t.Errorf("Updated book not found in GET /api/books response after update")
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/books/asd34343", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("DELETE /api/books/:id expected status %d, got %d", http.StatusOK, rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var booksAfterDelete []map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &booksAfterDelete)
	if err != nil {
		t.Fatalf("Error unmarshaling GET response after delete: %v", err)
	}

	for _, book := range booksAfterDelete {

		if book["ID"] == "asd34343" {
			t.Errorf("Deleted book (ID: %s) still present in GET /api/books response", book["ID"])
			break
		}
	}
}

func TestAuthorsEndpoint(t *testing.T) {
	setup()

	e.GET("/authors", func(c echo.Context) error {
		authors := findAuthors(coll)
		return c.JSON(http.StatusOK, authors)
	})

	testBooks := []BookStore{
		{ID: "a1", BookName: "Book1", BookAuthor: "author1", BookEdition: "1st", BookPages: "111", BookYear: "2020"},
		{ID: "a2", BookName: "Book2", BookAuthor: "author1", BookEdition: "2nd", BookPages: "222", BookYear: "2021"},
		{ID: "a3", BookName: "Book3", BookAuthor: "author2", BookEdition: "3rd", BookPages: "333", BookYear: "2022"},
	}

	for _, book := range testBooks {
		_, err := coll.InsertOne(context.TODO(), book)
		if err != nil {
			t.Fatalf("Failed to insert test book: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/authors", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /authors expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var authors []map[string]interface{}

	err := json.Unmarshal(rec.Body.Bytes(), &authors)
	if err != nil {
		t.Fatalf("Error unmarshaling GET response: %v", err)
	}
	fmt.Println(authors)
	if len(authors) != 6 {
		t.Errorf("Expected 6 authors, got %d", len(authors))
	}

	for _, author := range authors {
		name := author["author"].(string)
		books := author["books"].([]interface{})

		if name == "author1" {
			if len(books) != 2 {
				t.Errorf("Expected Test author1 to have 2 books, got %d", len(books))
			}
		} else if name == "author2" {
			if len(books) != 1 {
				t.Errorf("Expected Test author2 to have 1 book, got %d", len(books))
			}
			if books[0].(string) != "Book3" {
				t.Errorf("Expected 'Book3', got '%s'", books[0].(string))
			}
		}
	}

	_, err = coll.DeleteMany(context.TODO(), bson.M{"ID": bson.M{"$in": []string{"a1", "a2", "a3"}}})
	if err != nil {
		t.Logf("Failed to clean up test books: %v", err)
	}
}

func TestYearsEndpoint(t *testing.T) {
	setup()

	e.GET("/years", func(c echo.Context) error {
		years := findYears(coll)
		return c.JSON(http.StatusOK, years)
	})

	testBooks := []BookStore{
		{ID: "y1", BookName: "Book1", BookAuthor: "author1", BookEdition: "1st", BookPages: "111", BookYear: "2020"},
		{ID: "y2", BookName: "Book2", BookAuthor: "author2", BookEdition: "2nd", BookPages: "222", BookYear: "2020"},
		{ID: "y3", BookName: "Book3", BookAuthor: "author3", BookEdition: "3rd", BookPages: "333", BookYear: "2021"},
	}

	for _, book := range testBooks {
		_, err := coll.InsertOne(context.TODO(), book)
		if err != nil {
			t.Fatalf("Failed to insert test book: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/years", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /years expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var years []map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &years)
	if err != nil {
		t.Fatalf("Error unmarshaling GET response: %v", err)
	}

	if len(years) < 2 {
		t.Errorf("Expected at least 2 years, got %d", len(years))
	}

	year2020Found := false
	year2021Found := false

	for _, year := range years {
		yearVal := year["year"].(string)
		books := year["books"].([]interface{})

		if yearVal == "2020" {
			year2020Found = true
			if len(books) != 2 {
				t.Errorf("Expected year 2020 to have 2 books, got %d", len(books))
			}
			bookNames := make(map[string]bool)
			for _, book := range books {
				bookNames[book.(string)] = true
			}

			if !bookNames["Book1"] || !bookNames["Book2"] {
				t.Errorf("Missing expected books for year 2020")
			}
		} else if yearVal == "2021" {
			year2021Found = true
			if len(books) != 1 {
				t.Errorf("Expected year 2021 to have 1 book, got %d", len(books))
			}
			if books[0].(string) != "Book3" {
				t.Errorf("Expected 'Book3', got '%s'", books[0].(string))
			}
		}
	}

	if !year2020Found {
		t.Errorf("Year 2020 not found in the response")
	}
	if !year2021Found {
		t.Errorf("Year 2021 not found in the response")
	}

	_, err = coll.DeleteMany(context.TODO(), bson.M{"ID": bson.M{"$in": []string{"y1", "y2", "y3"}}})
	if err != nil {
		t.Logf("Failed to clean up test books: %v", err)
	}
}
