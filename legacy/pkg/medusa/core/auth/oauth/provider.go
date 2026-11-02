package oauth

// Configuraciones predefinidas para proveedores comunes
var (
	GoogleProvider = Provider{
		Name:        "google",
		UserInfoURL: "https://www.googleapis.com/oauth2/v2/userinfo",
		Scopes:      []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		FieldMapping: FieldMapping{
			ID:            "id",
			Email:         "email",
			Name:          "name",
			Picture:       "picture",
			VerifiedEmail: "verified_email",
		},
	}

	GitHubProvider = Provider{
		Name:        "github",
		UserInfoURL: "https://api.github.com/user",
		Scopes:      []string{"user:email"},
		FieldMapping: FieldMapping{
			ID:            "id",
			Email:         "email",
			Name:          "name",
			Picture:       "avatar_url",
			VerifiedEmail: "verified",
		},
	}

	FacebookProvider = Provider{
		Name:        "facebook",
		UserInfoURL: "https://graph.facebook.com/me?fields=id,name,email,picture",
		Scopes:      []string{"email", "public_profile"},
		FieldMapping: FieldMapping{
			ID:            "id",
			Email:         "email",
			Name:          "name",
			Picture:       "picture.data.url",
			VerifiedEmail: "verified",
		},
	}

	MicrosoftProvider = Provider{
		Name:        "microsoft",
		UserInfoURL: "https://graph.microsoft.com/v1.0/me",
		Scopes:      []string{"User.Read"},
		FieldMapping: FieldMapping{
			ID:            "id",
			Email:         "mail",
			Name:          "displayName",
			Picture:       "photo",
			VerifiedEmail: "verified",
		},
	}
)
