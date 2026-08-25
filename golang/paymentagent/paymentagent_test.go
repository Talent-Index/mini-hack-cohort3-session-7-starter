package paymentagent

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestFindOverdueInvoices(t *testing.T) {
	overdue := FindOverdueInvoices()
	if len(overdue) != 1 || overdue[0].ID != "INV-042" {
		t.Fatalf("expected only INV-042 overdue, got %+v", overdue)
	}
}

func TestPreflightChecks(t *testing.T) {
	inv := Invoices[0]
	if errs := PreflightChecks(inv); len(errs) != 0 {
		t.Fatalf("expected no preflight errors, got %v", errs)
	}

	bad := inv
	bad.AmountUSDC = 9999
	if errs := PreflightChecks(bad); len(errs) != 1 {
		t.Fatalf("expected 1 preflight error for oversized amount, got %v", errs)
	}
}

func TestERC20TransferData(t *testing.T) {
	data := erc20TransferData("0xCd34ef56ab12cd34ef56ab12cd34ef56ab12cd34", big.NewInt(50000000))
	if len(data) != 4+32+32 {
		t.Fatalf("expected 68 bytes of calldata, got %d", len(data))
	}
	if data[0] != 0xa9 || data[1] != 0x05 || data[2] != 0x9c || data[3] != 0xbb {
		t.Fatalf("expected transfer(address,uint256) selector 0xa9059cbb, got %x", data[:4])
	}
}

func TestKeyDerivationMatchesOtherLanguages(t *testing.T) {
	privKey, err := crypto.HexToECDSA("1111111111111111111111111111111111111111111111111111111111111111"[:64])
	if err != nil {
		t.Fatal(err)
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey)
	want := "0x19E7E376E7C213B7E7e7e46cc70A5dD086DAff2A"
	if addr.Hex() != want {
		t.Fatalf("address mismatch: got %s, want %s (this should match the JS/Python/Rust versions exactly)", addr.Hex(), want)
	}
}
