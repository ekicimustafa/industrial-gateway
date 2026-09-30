package modbus

// Note: decode() is unexported, so this test is in the same package (no _test suffix).
// This is called a "white-box test" in Go — it tests internal helpers directly.

import (
	"math"
	"testing"
)

func TestDecode_uint16(t *testing.T) {
	// Register value 2200 × scale 0.1 → 220.0 V
	data := []byte{0x08, 0x98} // big-endian 2200
	got, err := decode(data, DataUint16, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 220.0 {
		t.Errorf("uint16: want 220.0, got %v", got)
	}
}

func TestDecode_int16_negative(t *testing.T) {
	// Signed -100 × scale 1.0 → -100
	data := []byte{0xFF, 0x9C} // big-endian int16(-100)
	got, err := decode(data, DataInt16, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != -100.0 {
		t.Errorf("int16: want -100.0, got %v", got)
	}
}

func TestDecode_uint32(t *testing.T) {
	// 65536 × scale 0.001 → 65.536
	data := []byte{0x00, 0x01, 0x00, 0x00} // big-endian 65536
	got, err := decode(data, DataUint32, 0.001)
	if err != nil {
		t.Fatal(err)
	}
	want := 65.536
	if math.Abs(got.(float64)-want) > 1e-9 {
		t.Errorf("uint32: want %v, got %v", want, got)
	}
}

func TestDecode_float32(t *testing.T) {
	// IEEE 754 float32 3.14 encoded big-endian
	bits := math.Float32bits(3.14)
	data := []byte{byte(bits >> 24), byte(bits >> 16), byte(bits >> 8), byte(bits)}
	got, err := decode(data, DataFloat32, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.(float64)-float64(float32(3.14))) > 1e-6 {
		t.Errorf("float32: want ~3.14, got %v", got)
	}
}

func TestDecode_bool(t *testing.T) {
	on, _ := decode([]byte{0x01}, DataBool, 1.0)
	if on.(bool) != true {
		t.Error("bool: 0x01 should be true")
	}
	off, _ := decode([]byte{0x00}, DataBool, 1.0)
	if off.(bool) != false {
		t.Error("bool: 0x00 should be false")
	}
}

func TestDecode_scale_zero_treated_as_one(t *testing.T) {
	// scale=0 should behave like scale=1 (applyDefaults handles this but decode also guards it)
	data := []byte{0x00, 0x64} // 100
	got, err := decode(data, DataUint16, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 100.0 {
		t.Errorf("scale=0: want 100.0, got %v", got)
	}
}

func TestDecode_short_data_returns_error(t *testing.T) {
	_, err := decode([]byte{0x01}, DataUint16, 1.0) // need 2 bytes, got 1
	if err == nil {
		t.Error("expected error for short data, got nil")
	}
}

func TestDecode_unknown_type_returns_error(t *testing.T) {
	_, err := decode([]byte{0x01, 0x02}, "bad_type", 1.0)
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}
