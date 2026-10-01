package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func resetJWTSecret() {
	jwtSecret = nil
}

func TestInitJWT(t *testing.T) {
	t.Run("uses default secret when env unset", func(t *testing.T) {
		resetJWTSecret()
		orig, had := os.LookupEnv("JWT_SECRET")
		os.Unsetenv("JWT_SECRET")
		defer func() {
			if had {
				os.Setenv("JWT_SECRET", orig)
			}
		}()

		InitJWT()
		if string(jwtSecret) != "claude-code-cli-with-openai-api-default-secret-change-in-production" {
			t.Errorf("unexpected default secret: %q", jwtSecret)
		}
	})

	t.Run("uses JWT_SECRET env when set", func(t *testing.T) {
		resetJWTSecret()
		t.Setenv("JWT_SECRET", "my-custom-secret")
		InitJWT()
		if string(jwtSecret) != "my-custom-secret" {
			t.Errorf("secret = %q, want my-custom-secret", jwtSecret)
		}
	})
}

func TestGenerateAndValidateToken_RoundTrip(t *testing.T) {
	resetJWTSecret()
	t.Setenv("JWT_SECRET", "round-trip-secret")

	tokenString, err := GenerateToken("alice", 42, "admin")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if claims.Username != "alice" || claims.UserID != 42 || claims.Role != "admin" {
		t.Errorf("claims mismatch: %+v", claims)
	}
	if claims.ExpiresAt == nil || !claims.ExpiresAt.Time.After(time.Now()) {
		t.Errorf("expected ExpiresAt in the future, got %v", claims.ExpiresAt)
	}
}

func TestGenerateToken_LazyInitsSecretWhenUnset(t *testing.T) {
	resetJWTSecret()
	os.Unsetenv("JWT_SECRET")

	if _, err := GenerateToken("bob", 1, "user"); err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if len(jwtSecret) == 0 {
		t.Errorf("expected jwtSecret to be lazily initialized")
	}
}

func TestValidateToken_LazyInitsSecretWhenUnset(t *testing.T) {
	// 先用默认 secret 生成 token，再清空内存中的 jwtSecret，
	// 验证 ValidateToken 会自行懒加载出同样的默认 secret 从而校验成功。
	resetJWTSecret()
	os.Unsetenv("JWT_SECRET")
	tokenString, err := GenerateToken("carol", 2, "user")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	resetJWTSecret()
	claims, err := ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if claims.Username != "carol" {
		t.Errorf("Username = %q, want carol", claims.Username)
	}
}

func TestValidateToken_MalformedString(t *testing.T) {
	resetJWTSecret()
	t.Setenv("JWT_SECRET", "secret")
	if _, err := ValidateToken("not-a-jwt-token"); err == nil {
		t.Errorf("expected error for malformed token string")
	}
}

func TestValidateToken_WrongSecretFailsSignatureCheck(t *testing.T) {
	resetJWTSecret()
	t.Setenv("JWT_SECRET", "secret-a")
	tokenString, err := GenerateToken("dave", 3, "user")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	resetJWTSecret()
	t.Setenv("JWT_SECRET", "secret-b")
	if _, err := ValidateToken(tokenString); err == nil {
		t.Errorf("expected signature validation error when secret differs")
	}
}

// TestValidateToken_RejectsNonHMACSigningMethod 覆盖 keyFunc 里对签名算法的
// 校验分支：ValidateToken 只信任 HMAC 系列算法，收到用其它算法（这里用
// jwt 库显式允许的 "none" 算法构造未签名 token）签发的 token 时必须拒绝，
// 否则会被经典的 alg=none 攻击绕过签名校验。
func TestValidateToken_RejectsNonHMACSigningMethod(t *testing.T) {
	resetJWTSecret()
	t.Setenv("JWT_SECRET", "secret")

	token := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{Username: "eve"})
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to build unsigned token: %v", err)
	}

	if _, err := ValidateToken(tokenString); err == nil {
		t.Errorf("expected error for non-HMAC signing method")
	}
}

func TestValidateToken_RejectsRSASignedToken(t *testing.T) {
	resetJWTSecret()
	t.Setenv("JWT_SECRET", "secret")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, &Claims{Username: "frank"})
	tokenString, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign RS256 token: %v", err)
	}

	if _, err := ValidateToken(tokenString); err == nil {
		t.Errorf("expected error for RS256-signed token against HMAC validator")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	resetJWTSecret()
	t.Setenv("JWT_SECRET", "secret")
	InitJWT()

	claims := &Claims{
		Username: "grace",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("failed to sign expired token: %v", err)
	}

	if _, err := ValidateToken(tokenString); err == nil {
		t.Errorf("expected error for expired token")
	}
}
