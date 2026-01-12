package oauth

// User representa la información básica de un usuario de cualquier proveedor
type User struct {
	ID            string                 `json:"id"`
	Email         string                 `json:"email"`
	Name          string                 `json:"name"`
	Picture       string                 `json:"picture"`
	VerifiedEmail bool                   `json:"verified_email"`
	RawData       map[string]interface{} `json:"raw_data"` // Datos adicionales específicos del proveedor
}

// Provider contiene la configuración específica de cada proveedor
type Provider struct {
	Name         string
	UserInfoURL  string
	Scopes       []string
	FieldMapping FieldMapping
}

// FieldMapping mapea los campos del proveedor a nuestros campos genéricos
type FieldMapping struct {
	ID            string
	Email         string
	Name          string
	Picture       string
	VerifiedEmail string
}
