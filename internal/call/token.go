package call

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type videoGrant struct {
	RoomJoin     bool   `json:"roomJoin,omitempty"`
	Room         string `json:"room,omitempty"`
	CanPublish   bool   `json:"canPublish"`
	CanSubscribe bool   `json:"canSubscribe"`
}

type accessTokenClaims struct {
	Video videoGrant `json:"video"`
	jwt.RegisteredClaims
}

// mintToken builds a LiveKit access token by hand instead of depending on
// github.com/livekit/protocol, which drags in pion/webrtc, grpc, and a dozen
// other transitive packages just for this one JWT-signing helper. The claim
// shape matches LiveKit's documented access token format: an HS256 JWT with
// iss=apiKey, sub=identity, and a "video" grant object, signed with apiSecret.
func mintToken(apiKey, apiSecret, identity, roomName string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := accessTokenClaims{
		Video: videoGrant{RoomJoin: true, Room: roomName, CanPublish: true, CanSubscribe: true},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    apiKey,
			Subject:   identity,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(apiSecret))
}
