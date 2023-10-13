package utils

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"unicode"

	zap "go.uber.org/zap"
	zapcore "go.uber.org/zap/zapcore"
	"golang.org/x/crypto/bcrypt"
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

func HashPassword(password string, cost int) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}

	return string(hashedPassword), nil
}

func VerifyPassword(storedPassword string, inputPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(inputPassword))

	return err == nil
}

func IsPasswordStrong(password string) bool {
	// Check the length of the password
	if len(password) < 8 {
		return false
	}

	// Check if the password contains at least one uppercase letter
	containsUppercase := false
	for _, char := range password {
		if unicode.IsUpper(char) {
			containsUppercase = true
			break
		}
	}
	if !containsUppercase {
		return false
	}

	// Check if the password contains at least one lowercase letter
	containsLowercase := false
	for _, char := range password {
		if unicode.IsLower(char) {
			containsLowercase = true
			break
		}
	}
	if !containsLowercase {
		return false
	}

	// Check if the password contains at least one digit
	containsDigit := false
	for _, char := range password {
		if unicode.IsDigit(char) {
			containsDigit = true
			break
		}
	}
	if !containsDigit {
		return false
	}

	// Check if the password contains at least one special character
	containsSpecial := false
	for _, char := range password {
		if unicode.IsPunct(char) || unicode.IsSymbol(char) {
			containsSpecial = true
			break
		}
	}

	return containsSpecial
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

func GenericInsert[model any](table string, data []byte, db *sql.DB) error {
	var body model

	err := json.Unmarshal(data, &body)
	if err != nil {
		return err
	}

	var params model
	fields := GetDBFieldNames(reflect.TypeOf(params))
	values := GetStructFieldValues(body)

	fields = append(fields[:0], fields[1:]...)
	values = append(values[:0], values[1:]...)

	fieldString := strings.Join(fields, ",")
	valuePlaceHolders := strings.Repeat("?, ", len(values)-1) + "?"

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, fieldString, valuePlaceHolders)

	_, err = db.Exec(query, values...)
	if err != nil {
		return err
	}
	return nil
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
