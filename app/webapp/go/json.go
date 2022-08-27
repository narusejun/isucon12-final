package main

import (
	"fmt"
	"net/http"

	"github.com/goccy/go-json"
	"github.com/labstack/echo/v4"
)

type JsonSerializer struct{}

func (d JsonSerializer) Serialize(c echo.Context, i interface{}, _ string) error {
	return json.NewEncoder(c.Response()).Encode(i)
}

func (d JsonSerializer) Deserialize(c echo.Context, i interface{}) error {
	err := json.NewDecoder(c.Request().Body).Decode(i)
	if ute, ok := err.(*json.UnmarshalTypeError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, field=%v, offset=%v", ute.Type, ute.Value, ute.Field, ute.Offset)).SetInternal(err)
	} else if se, ok := err.(*json.SyntaxError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Syntax error: offset=%v, error=%v", se.Offset, se.Error())).SetInternal(err)
	}
	return err
}
