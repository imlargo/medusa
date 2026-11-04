package medusa

import (
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"github.com/go-playground/validator/v10"

	"github.com/imlargo/medusa/apperr"
)

// ---------------------------------------------------------------------------
// Binding
// ---------------------------------------------------------------------------

// Bind binds and validates the request body to the given struct.
func (c *Context) Bind(obj any) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		return bindError(err, obj, "json", ErrMalformedBody)
	}
	return nil
}

// BindQuery binds and validates query parameters.
func (c *Context) BindQuery(obj any) error {
	if err := c.ShouldBindQuery(obj); err != nil {
		return bindError(err, obj, "form", ErrInvalidParameter)
	}
	return nil
}

// BindURI binds and validates URI parameters.
func (c *Context) BindURI(obj any) error {
	if err := c.ShouldBindUri(obj); err != nil {
		return bindError(err, obj, "uri", ErrInvalidParameter)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Validation error formatting
// ---------------------------------------------------------------------------

// bindError turns a binding failure into a client-facing error: ErrValidation
// listing each rejected field by the name the client sent (its tag in obj),
// or unparsable when the input could not be decoded at all.
func bindError(err error, obj any, tag string, unparsable *apperr.Error) error {
	// A body cut off by the size cap is not a validation problem, and reporting
	// it as one sends the client hunting for a field that is fine.
	if tooLarge, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return ErrBodyTooLarge.WithMeta("max_bytes", tooLarge.Limit).WithCause(err)
	}

	if validationErrors, ok := errors.AsType[validator.ValidationErrors](err); ok {
		fields := make([]apperr.FieldError, 0, len(validationErrors))
		for _, fieldErr := range validationErrors {
			fields = append(fields, apperr.FieldError{
				Field:   fieldPath(reflect.TypeOf(obj), fieldErr.StructNamespace(), tag),
				Message: formatValidationMessage(fieldErr),
			})
		}
		slices.SortFunc(fields, func(a, b apperr.FieldError) int { return strings.Compare(a.Field, b.Field) })
		return ErrValidation.WithFields(fields...)
	}

	return unparsable.WithCause(err)
}

// fieldPath turns a validator namespace such as "Order.Items[0].UnitPrice"
// into the path the client wrote, "items[0].unit_price", by reading each
// field's tag in t. A field without the tag keeps its Go name.
func fieldPath(t reflect.Type, namespace, tag string) string {
	segments := strings.Split(namespace, ".")[1:] // the first one is the type name
	path := make([]string, 0, len(segments))

	for _, segment := range segments {
		name, index, _ := strings.Cut(segment, "[")
		if index != "" {
			index = "[" + index
		}

		for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map) {
			t = t.Elem()
		}

		label := name
		if t != nil && t.Kind() == reflect.Struct {
			if field, ok := t.FieldByName(name); ok {
				if tagged, _, _ := strings.Cut(field.Tag.Get(tag), ","); tagged != "" && tagged != "-" {
					label = tagged
				}
				t = field.Type
			} else {
				t = nil
			}
		}

		path = append(path, label+index)
	}

	return strings.Join(path, ".")
}

func formatValidationMessage(e validator.FieldError) string {
	kind := e.Type().Kind()

	switch e.Tag() {
	case "required":
		return "this field is required"
	case "email":
		return "invalid email format"

	case "min":
		switch kind {
		case reflect.Slice, reflect.Array, reflect.Map:
			return "must have at least " + e.Param() + " elements"
		case reflect.String:
			return "must be at least " + e.Param() + " characters"
		default:
			return "must be at least " + e.Param()
		}

	case "max":
		switch kind {
		case reflect.Slice, reflect.Array, reflect.Map:
			return "must have at most " + e.Param() + " elements"
		case reflect.String:
			return "must be at most " + e.Param() + " characters"
		default:
			return "must be at most " + e.Param()
		}

	case "len":
		switch kind {
		case reflect.Slice, reflect.Array, reflect.Map:
			return "must have exactly " + e.Param() + " elements"
		case reflect.String:
			return "must be exactly " + e.Param() + " characters"
		default:
			return "must have length " + e.Param()
		}

	case "gte":
		return "must be greater than or equal to " + e.Param()
	case "lte":
		return "must be less than or equal to " + e.Param()
	case "gt":
		return "must be greater than " + e.Param()
	case "lt":
		return "must be less than " + e.Param()
	case "eq":
		return "must be equal to " + e.Param()
	case "ne":
		return "must not be equal to " + e.Param()
	case "oneof":
		return "must be one of: " + e.Param()

	case "url":
		return "invalid URL format"
	case "uuid":
		return "invalid UUID format"
	case "uuid4":
		return "invalid UUID v4 format"

	case "alpha":
		return "must contain only letters"
	case "alphanum":
		return "must contain only letters and numbers"
	case "numeric":
		return "must be a valid number"

	case "contains":
		return "must contain '" + e.Param() + "'"
	case "containsany":
		return "must contain at least one of: " + e.Param()
	case "excludes":
		return "must not contain '" + e.Param() + "'"
	case "startswith":
		return "must start with '" + e.Param() + "'"
	case "endswith":
		return "must end with '" + e.Param() + "'"

	case "json":
		return "must be valid JSON"
	case "jwt":
		return "must be a valid JWT token"
	case "datetime":
		return "must be a valid datetime in format: " + e.Param()

	case "ip":
		return "must be a valid IP address"
	case "ipv4":
		return "must be a valid IPv4 address"
	case "ipv6":
		return "must be a valid IPv6 address"

	case "latitude":
		return "must be a valid latitude"
	case "longitude":
		return "must be a valid longitude"

	default:
		return "invalid value"
	}
}

// ---------------------------------------------------------------------------
// URL parameter helpers
// ---------------------------------------------------------------------------

// ParamID gets a URL parameter as a positive int64, the range of a Postgres
// bigint key. Returns an error if the parameter is missing, non-numeric, or
// not greater than zero.
func (c *Context) ParamID(name string) (int64, error) {
	param := c.Param(name)
	if param == "" {
		return 0, paramError(name, "this parameter is required")
	}

	id, err := strconv.ParseInt(param, 10, 64)
	if err != nil || id <= 0 {
		return 0, paramError(name, "must be a positive integer")
	}

	return id, nil
}

// ParamUUID extracts a URL parameter and validates it as a UUID. The returned
// string is normalized to the canonical lowercase form.
func (c *Context) ParamUUID(name string) (string, error) {
	param := c.Param(name)
	if param == "" {
		return "", paramError(name, "this parameter is required")
	}

	parsed, err := uuid.Parse(param)
	if err != nil {
		return "", paramError(name, "must be a valid UUID")
	}

	return parsed.String(), nil
}

// paramError reports the URL parameter name as invalid.
func paramError(name, message string) error {
	return ErrInvalidParameter.WithFields(apperr.FieldError{Field: name, Message: message})
}

// ---------------------------------------------------------------------------
// Typed query-parameter helpers
// ---------------------------------------------------------------------------

// queryParse is the shared backbone for all typed Query* helpers.
// It reads the named query parameter and parses it with fn.
// On any error (missing or unparseable) it returns defaultValue.
func queryParse[T any](c *Context, name string, defaultValue T, fn func(string) (T, error)) T {
	val := c.Query(name)
	if val == "" {
		return defaultValue
	}
	parsed, err := fn(val)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// QueryInt gets a query parameter as int. Returns defaultValue when the
// parameter is missing or cannot be parsed.
func (c *Context) QueryInt(name string, defaultValue int) int {
	return queryParse(c, name, defaultValue, strconv.Atoi)
}

// QueryInt64 gets a query parameter as int64. Returns defaultValue when the
// parameter is missing or cannot be parsed.
func (c *Context) QueryInt64(name string, defaultValue int64) int64 {
	return queryParse(c, name, defaultValue, func(s string) (int64, error) {
		return strconv.ParseInt(s, 10, 64)
	})
}

// QueryUint gets a query parameter as uint. Returns defaultValue when the
// parameter is missing or cannot be parsed.
func (c *Context) QueryUint(name string, defaultValue uint) uint {
	return queryParse(c, name, defaultValue, func(s string) (uint, error) {
		v, err := strconv.ParseUint(s, 10, 64)
		return uint(v), err
	})
}

// QueryBool gets a query parameter as bool. Returns defaultValue when the
// parameter is missing or cannot be parsed.
func (c *Context) QueryBool(name string, defaultValue bool) bool {
	return queryParse(c, name, defaultValue, strconv.ParseBool)
}

// QueryFloat64 gets a query parameter as float64. Returns defaultValue when
// the parameter is missing or cannot be parsed.
func (c *Context) QueryFloat64(name string, defaultValue float64) float64 {
	return queryParse(c, name, defaultValue, func(s string) (float64, error) {
		return strconv.ParseFloat(s, 64)
	})
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

// Pagination constants for consistent default values.
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
	MinPageSize     = 1
)

// Pagination represents pagination parameters extracted from the query string.
// Validation against MinPageSize / MaxPageSize is enforced both at the binding level
// (via binding tags) and programmatically in GetPageSize to ensure consistency.
type Pagination struct {
	Page     int `form:"page"      binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1"`
}

// GetPage returns the validated page number (minimum 1).
func (p Pagination) GetPage() int {
	if p.Page < DefaultPage {
		return DefaultPage
	}
	return p.Page
}

// GetPageSize returns a validated page size clamped to [MinPageSize, MaxPageSize].
// When p.PageSize is not set (< MinPageSize) the provided defaultSize is used,
// itself clamped to the same range.
func (p Pagination) GetPageSize(defaultSize int) int {
	defaultSize = clampPageSize(defaultSize)

	if p.PageSize < MinPageSize {
		return defaultSize
	}
	return clampPageSize(p.PageSize)
}

// Offset calculates the SQL OFFSET for the current page.
func (p Pagination) Offset(defaultSize int) int {
	return (p.GetPage() - 1) * p.GetPageSize(defaultSize)
}

// clampPageSize sanitises a page-size value:
//   - values ≤ 0 (unset / invalid) → DefaultPageSize
//   - values > MaxPageSize          → MaxPageSize
//   - everything else               → unchanged
func clampPageSize(v int) int {
	if v < MinPageSize {
		return DefaultPageSize
	}
	if v > MaxPageSize {
		return MaxPageSize
	}
	return v
}

// Pagination extracts pagination params from the query string and uses
// BindQuery so that struct validation tags are honoured.
func (c *Context) Pagination() (Pagination, error) {
	var p Pagination
	if err := c.BindQuery(&p); err != nil {
		return Pagination{}, err
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Sorting
// ---------------------------------------------------------------------------

// SortOrder represents the sorting direction.
type SortOrder string

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)

// SortParams holds the resolved sorting field and direction.
type SortParams struct {
	Field string
	Order SortOrder
}

// Sort extracts sorting parameters from the query string. The field usually
// ends up in an ORDER BY, so only names in allowedFields are accepted; any
// other sort_by falls back to defaultField. With no allowedFields, only
// defaultField is. A defaultField missing from allowedFields is replaced by
// the first allowed field.
//
// Example: ?sort_by=created_at&sort_order=desc
func (c *Context) Sort(defaultField string, defaultOrder SortOrder, allowedFields ...string) SortParams {
	if len(allowedFields) == 0 {
		allowedFields = []string{defaultField}
	}

	field := c.Query("sort_by")
	if !slices.Contains(allowedFields, field) {
		field = defaultField
		if !slices.Contains(allowedFields, field) {
			field = allowedFields[0]
		}
	}

	order := SortOrder(c.Query("sort_order"))
	if order != SortOrderAsc && order != SortOrderDesc {
		order = defaultOrder
	}

	return SortParams{
		Field: field,
		Order: order,
	}
}
