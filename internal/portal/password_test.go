package portal

import "testing"

func TestEncryptPasswordRustVector(t *testing.T) {
	// The expected ciphertext is copied from the Rust implementation's test.
	const input = "testPassword123>aabbccddeeff"
	const want = "71292489a26b05325b8498d89cc4381a02712b3c6d7c5a57f032e9a65578e66b1131b508f163cfb0dcc920439cb229302ddda8034aeca055a89f9fdb026f5f3ba64d4885f4c332621949cdfe8c2cfae2a736b55585823e2e1082f236c9f0def4bd3987b1e90a54cc710832ad61b6951311e223638a3cc5fd8f11c78b50c07318"
	got, err := EncryptPassword(input)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ciphertext differs from Rust vector\ngot:  %s\nwant: %s", got, want)
	}
}
