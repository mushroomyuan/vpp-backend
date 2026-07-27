package types

import (
	"strings"
	"testing"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

func TestAssetItem_Validate(t *testing.T) {
	t.Parallel()

	ok := AssetItem{Name: "bess"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (AssetItem{}).Validate(); err == nil || !strings.Contains(err.Error(), "Name") {
		t.Fatalf("empty name: %v", err)
	}

	empty := ""
	neg := -1.0
	cases := []AssetItem{
		{Name: "x", SubType: &empty},
		{Name: "x", DispatchMode: &empty},
		{Name: "x", EnergyType: &empty},
		{Name: "x", OwnerType: &empty},
		{Name: "x", Description: &empty},
		{Name: "x", RatedCapacityKW: &neg},
	}
	for i, item := range cases {
		if err := item.Validate(); err == nil {
			t.Fatalf("case %d: want error", i)
		}
	}
}

func TestCUItem_Validate(t *testing.T) {
	t.Parallel()

	if err := (CUItem{Name: "cu", Type: "modbus"}).Validate(); err != nil {
		t.Fatal(err)
	}
	err := (CUItem{}).Validate()
	if err == nil || !strings.Contains(err.Error(), "Name") || !strings.Contains(err.Error(), "Type") {
		t.Fatalf("want both missing fields: %v", err)
	}
}

func TestPointItem_Validate(t *testing.T) {
	t.Parallel()

	if err := (PointItem{PointKey: "p", DataType: model.DataTypeInt}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (PointItem{DataType: model.DataTypeInt}).Validate(); err == nil {
		t.Fatal("missing PointKey")
	}
	if err := (PointItem{PointKey: "p", DataType: "Nope"}).Validate(); err == nil {
		t.Fatal("invalid DataType")
	}
}
