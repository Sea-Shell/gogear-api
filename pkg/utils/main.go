package utils

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
	zap "go.uber.org/zap"
	zapcore "go.uber.org/zap/zapcore"
	yaml "gopkg.in/yaml.v2"
)

func GetLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warning":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	default:
		return zap.InfoLevel // Default to info level if an invalid log level is specified
	}
}

func CheckApiKey() gin.HandlerFunc {
	return func(c *gin.Context) {

		token := c.Request.Header.Get("X-API-Key")

		if token == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "Token must be set"})
			return
		}
		if token != "cliff-manana-crocus-canard" {
			c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "Token is invalid"})
			return
		}

		c.Next()
	}
}

func LoadConfig[config any](filename string) (*config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var conf config

	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, err
	}

	return &conf, nil
}

func GetDBFieldNames(structType reflect.Type) []string {
	var fieldNames []string

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		dbFieldName := field.Tag.Get("db")
		if dbFieldName != "" {
			fieldNames = append(fieldNames, dbFieldName)
		}
	}

	return fieldNames
}

func GetStructFieldValues(s interface{}) []interface{} {
	structValue := reflect.ValueOf(s)

	values := make([]interface{}, 0, structValue.NumField())

	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		values = append(values, field.Interface())
	}

	return values
}

func GetScanFields(v interface{}) ([]interface{}, error) {
	valueType := reflect.TypeOf(v)

	if valueType.Kind() != reflect.Struct {
		return nil, errors.New("input must be a struct")
	}

	dest := make([]interface{}, valueType.NumField())

	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)

		dest[i] = reflect.New(field.Type).Interface()
	}

	return dest, nil
}

func GenericUpdate[model any](table string, data []byte, db *sql.DB) error {
	var body model

	err := json.Unmarshal(data, &body)
	if err != nil {
		return err
	}

	v := reflect.ValueOf(body)
	if v.NumField() == 0 {
		return err
	}

	fieldValue := v.Field(0).Interface()
	idValue := reflect.Indirect(reflect.ValueOf(fieldValue)).Interface()

	var fields []string = GetDBFieldNames(reflect.TypeOf(body))
	var updateFields []string
	var updateValues []interface{}

	for i, field := range fields {
		// Exclude the first field (ID) from the updateFields to avoid modifying it
		if i == 0 {
			continue
		}
		updateFields = append(updateFields, field+" = ?")
		updateValues = append(updateValues, v.Field(i).Interface())
	}

	updateFieldsClause := strings.Join(updateFields, ", ")

	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", table, updateFieldsClause, fields[0])

	updateValues = append(updateValues, idValue)

	_, err = db.Exec(query, updateValues...)
	if err != nil {
		return err
	}

	return nil
}

func GenericInsert[model any](table string, data []byte, db *sql.DB) (*model, error) {
	var body model

	err := json.Unmarshal(data, &body)
	if err != nil {
		return nil, err
	}

	var params model
	fields := GetDBFieldNames(reflect.TypeOf(params))
	values := GetStructFieldValues(body)

	fields = append(fields[:0], fields[1:]...)
	values = append(values[:0], values[1:]...)

	fieldString := strings.Join(fields, ",")
	valuePlaceHolders := strings.Repeat("?, ", len(values)-1) + "?"

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, fieldString, valuePlaceHolders)

	results, err := db.Exec(query, values...)
	if err != nil {
		return nil, err
	}

	lastID, err := results.LastInsertId()
	if err != nil {
		return nil, err
	}

	createdObject, err := GenericGet[model](table, int(lastID), nil, db)
	if err != nil {
		return nil, err
	}
	return createdObject, nil
}

func GenericGet[model any](table string, id int, sql []string, db *sql.DB) (*model, error) {

	var params model

	fields := GetDBFieldNames(reflect.TypeOf(params))

	extraSql := ""
	if len(sql) > 0 {
		extraSql = " " + strings.Join(sql, " ")
	}

	baseQuery := fmt.Sprintf("SELECT %s FROM %s ", strings.Join(fields, ", "), table)

	whereClause := fmt.Sprintf("WHERE %s = ?", fields[0])

	queryLimit := "LIMIT 1"

	query := baseQuery + extraSql + whereClause + queryLimit

	row := db.QueryRow(query, id)

	dest, err := GetScanFields(params)
	if err != nil {
		return nil, err
	}

	err = row.Scan(dest...)
	if err != nil {
		return nil, err
	}

	for i := 0; i < reflect.TypeOf(params).NumField(); i++ {

		reflect.ValueOf(&params).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
	}

	return &params, nil
}

func GenericDelete[model any](table string, id int, db *sql.DB) (*model, error) {

	var params model

	fields := GetDBFieldNames(reflect.TypeOf(params))

	if !strings.Contains(strings.ToLower(fields[0]), "id") {
		return nil, errors.New("Invalid field order in struct. first field must be id")
	}

	deletedObject, err := GenericGet[model](table, id, nil, db)
	if err != nil {
		return nil, err
	}

	baseQuery := fmt.Sprintf("DELETE FROM %s ", table)

	whereClause := fmt.Sprintf("WHERE %s = ?", fields[0])

	query := baseQuery + whereClause

	result, err := db.Exec(query, id)
	if err != nil {
		return nil, err
	}

	row, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if row == 0 {
		return nil, errors.New("No rows affected")
	} else {
		return deletedObject, nil
	}
}

func GenericList[model any](table string, field string, id int, db *sql.DB) (*[]model, error) {
	var params model

	fields := GetDBFieldNames(reflect.TypeOf(params))

	var modelFieldCount int = 0
	var fieldIndexNumber int

	for i, modelField := range fields {
		if modelField == field {
			modelFieldCount += 1
			fieldIndexNumber = i
		}
	}
	if modelFieldCount == 0 {
		errorMsg := fmt.Sprintf("Invalid field supplied to GenericList function %v is not defined in model supplied %#v", field, fields)
		return nil, errors.New(errorMsg)
	}

	baseQuery := fmt.Sprintf("SELECT %s FROM %s ", strings.Join(fields, ", "), table)

	whereClause := fmt.Sprintf("WHERE %s = ?", fields[fieldIndexNumber])

	query := baseQuery + whereClause

	rows, err := db.Query(query, id)
	if err != nil {
		errorMsg := fmt.Sprintf("Query error: %#v", err.Error())
		return nil, errors.New(errorMsg)
	}

	dest, err := GetScanFields(params)
	if err != nil {
		return nil, err
	}

	var genericObjectList []model

	for rows.Next() {
		err = rows.Scan(dest...)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, errors.New("Error: No results")
			}
			errorMsg := fmt.Sprintf("Scan error: %#v", err)
			return nil, errors.New(errorMsg)
		}

		for i := 0; i < reflect.TypeOf(params).NumField(); i++ {
			reflect.ValueOf(&params).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
		}

		genericObjectList = append(genericObjectList, params)
	}

	return &genericObjectList, nil
}
