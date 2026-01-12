package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

// OAuth2Authenticator es el autenticador genérico para OAuth2
type OAuth2Authenticator struct {
	oauthConfig    *oauth2.Config
	providerConfig Provider
}

// NewGenericAuthenticator crea un nuevo autenticador genérico
func NewGenericAuthenticator(oauthConfig *oauth2.Config, provider Provider) *OAuth2Authenticator {
	// Asignar los scopes del proveedor si no están definidos
	if len(oauthConfig.Scopes) == 0 {
		oauthConfig.Scopes = provider.Scopes
	}

	return &OAuth2Authenticator{
		oauthConfig:    oauthConfig,
		providerConfig: provider,
	}
}

// GetUser obtiene la información del usuario desde cualquier proveedor OAuth2
func (a *OAuth2Authenticator) GetUser(token *oauth2.Token) (*User, error) {
	// Crear cliente HTTP con el token de acceso
	client := a.oauthConfig.Client(context.Background(), token)

	// Realizar la petición al endpoint de información del usuario
	resp, err := client.Get(a.providerConfig.UserInfoURL)
	if err != nil {
		return nil, fmt.Errorf("error retrieving user information from %s: %v", a.providerConfig.Name, err)
	}
	defer resp.Body.Close()

	// Verificar el código de estado
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error in %s API response: %s", a.providerConfig.Name, resp.Status)
	}

	// Decodificar la respuesta en un mapa genérico
	var rawData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return nil, fmt.Errorf("error decoding user information: %v", err)
	}

	// Mapear los campos al usuario genérico
	user := &User{
		RawData: rawData,
	}

	user.ID = a.extractField(rawData, a.providerConfig.FieldMapping.ID)
	user.Email = a.extractField(rawData, a.providerConfig.FieldMapping.Email)
	user.Name = a.extractField(rawData, a.providerConfig.FieldMapping.Name)
	user.Picture = a.extractField(rawData, a.providerConfig.FieldMapping.Picture)

	// Manejar VerifiedEmail (puede ser bool o no existir)
	if verifiedField := a.providerConfig.FieldMapping.VerifiedEmail; verifiedField != "" {
		if verified, ok := rawData[verifiedField].(bool); ok {
			user.VerifiedEmail = verified
		}
	}

	return user, nil
}

// extractField extrae un campo del mapa de datos raw (soporta campos anidados con notación de punto)
func (a *OAuth2Authenticator) extractField(data map[string]interface{}, fieldPath string) string {
	if fieldPath == "" {
		return ""
	}

	// Para campos simples
	if val, ok := data[fieldPath]; ok {
		return fmt.Sprint(val)
	}

	// Para campos anidados como "picture.data.url"
	// Esta es una implementación simple, podrías hacerla más robusta
	return ""
}

// GetOAuthConfig retorna la configuración de OAuth2
func (a *OAuth2Authenticator) GetOAuthConfig() *oauth2.Config {
	return a.oauthConfig
}

// GetAuthURL genera la URL de autenticación
func (a *OAuth2Authenticator) GetAuthURL(state string) string {
	return a.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeCode intercambia el código de autorización por un token
func (a *OAuth2Authenticator) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := a.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("error exchanging code for token: %v", err)
	}
	return token, nil
}
