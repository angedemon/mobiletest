package results

import (
	"testing"
	"time"
)

func TestVerifyGoogleToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	valid := map[string]any{
		"aud": "yele-web.apps.googleusercontent.com", "iss": "https://accounts.google.com",
		"exp": "1800000600", "email_verified": "true", "email": "awa@gmail.com",
		"sub": "1234567890", "name": "Awa",
	}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range valid {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	defer func(f func(string) (map[string]any, error)) { googleTokenInfo = f }(googleTokenInfo)

	cases := []struct {
		name    string
		info    map[string]any
		wantErr bool
	}{
		{"jeton valide", valid, false},
		{"autre application (audience)", with("aud", "autre-app.apps.googleusercontent.com"), true},
		{"émetteur étranger", with("iss", "evil.example.com"), true},
		{"jeton expiré", with("exp", "1799999999"), true},
		{"e-mail non vérifié", with("email_verified", "false"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := c.info
			googleTokenInfo = func(string) (map[string]any, error) { return info, nil }
			id, err := verifyGoogleToken("jeton",
				"yele-android.apps.googleusercontent.com, yele-web.apps.googleusercontent.com", now)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, erreur attendue : %v", err, c.wantErr)
			}
			if err == nil && (id.Sub != "1234567890" || id.Email != "awa@gmail.com") {
				t.Errorf("identité inattendue : %+v", id)
			}
		})
	}

	if _, err := verifyGoogleToken("jeton", "", now); err == nil {
		t.Error("sans identifiant client configuré, la connexion Google doit être refusée")
	}
}
