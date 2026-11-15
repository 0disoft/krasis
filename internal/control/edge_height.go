package control

import (
	"context"
	"errors"
	"math"
)

// EdgeHeight is a normalized vertical crossing coordinate, independent of DPI
// and resolution. Zero means unspecified; 1 and 65536 are the top and bottom.
type EdgeHeight uint32

const edgeHeightScale int64 = 65535

func (h EdgeHeight) Valid() bool { return h <= EdgeHeight(edgeHeightScale+1) }

func NormalizeEdgeHeight(y, top, height int32) EdgeHeight {
	if height <= 0 || int64(top)+int64(height)-1 > math.MaxInt32 {
		return 0
	}
	if height == 1 {
		return 1
	}
	span := int64(height) - 1
	offset := max(int64(0), min(int64(y)-int64(top), span))
	return EdgeHeight((offset*edgeHeightScale+span/2)/span + 1)
}

func (h EdgeHeight) Coordinate(top, height int32) (int32, error) {
	if !h.Valid() || height <= 0 || int64(top)+int64(height)-1 > math.MaxInt32 {
		return 0, errors.New("invalid edge height or screen geometry")
	}
	if h == 0 { // Preserve legacy center placement exactly.
		return int32(int64(top) + int64(height)/2), nil
	}
	return int32(int64(top) + (int64(h-1)*(int64(height)-1)+edgeHeightScale/2)/edgeHeightScale), nil
}

type edgeHeightContextKey struct{}

// WithEdgeHeight carries presentation data through existing authorized ports;
// it never provides a lease, changes admission, or authorizes native input.
func WithEdgeHeight(ctx context.Context, h EdgeHeight) context.Context {
	return context.WithValue(ctx, edgeHeightContextKey{}, h)
}

func EdgeHeightFromContext(ctx context.Context) EdgeHeight {
	if ctx == nil {
		return 0
	}
	h, _ := ctx.Value(edgeHeightContextKey{}).(EdgeHeight)
	return h
}
