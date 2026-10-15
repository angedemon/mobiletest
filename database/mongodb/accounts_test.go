package mongodb

import "testing"

func TestHashPasswordDeterministicPerSalt(t *testing.T) {
	a, _ := hashPassword("motdepasse", []byte("sel-1-sel-1-sel1"))
	b, _ := hashPassword("motdepasse", []byte("sel-1-sel-1-sel1"))
	c, _ := hashPassword("motdepasse", []byte("sel-2-sel-2-sel2"))
	if a != b {
		t.Error("même mot de passe et même sel : hachages différents")
	}
	if a == c {
		t.Error("sels différents : hachages identiques")
	}
}

func TestTokenHashHidesToken(t *testing.T) {
	if tokenHash("abc") == "abc" || len(tokenHash("abc")) != 64 {
		t.Error("l'empreinte du jeton doit être un SHA-256 hexadécimal")
	}
}

func TestNormalizeEmail(t *testing.T) {
	if NormalizeEmail("  Awa@Exemple.BF ") != "awa@exemple.bf" {
		t.Error("e-mail mal normalisé")
	}
}
