package matrix
import (
	"testing"
)
func TestMatrixRow1_CalendarOrder(t *testing.T) {
	ctx := DocumentContext{
		Kind:  KindOrder,
		Title: "Приказ о рабочем календаре и нерабочих праздничных днях",
	}
	res := CalculateVisas(ctx)
	hasLegal := false
	hasHR := false
	hasDOU := false
	for _, v := range res.RequiredVisas {
		if v.Department == DeptLegal {
			hasLegal = true
		}
		if v.Department == DeptHR {
			hasHR = true
		}
		if v.Department == DeptDOU {
			hasDOU = true
		}
	}
	if !hasLegal || !hasHR || !hasDOU {
		t.Errorf("Expected Legal, HR and DOU visas for calendar order, got: %+v", res.RequiredVisas)
	}
}
func TestMatrixRow2_PropertyTransfer_SpecialInfoSecCondition(t *testing.T) {
	// Case A: No IS access change -> InfoSec NOT required
	ctxA := DocumentContext{
		Kind:              KindOrder,
		Title:             "Приказ о приеме и передаче дел и имущества",
		HasISAccessChange: false,
		MatchedModuleID:   "M-03",
	}
	resA := CalculateVisas(ctxA)
	for _, v := range resA.RequiredVisas {
		if v.Department == DeptInfoSec {
			t.Errorf("Did not expect InfoSec visa when HasISAccessChange is false")
		}
	}
	// Case B: IS access change -> InfoSec IS REQUIRED
	ctxB := DocumentContext{
		Kind:              KindOrder,
		Title:             "Приказ о приеме и передаче дел и имущества",
		HasISAccessChange: true,
		MatchedModuleID:   "M-03",
	}
	resB := CalculateVisas(ctxB)
	hasInfoSec := false
	for _, v := range resB.RequiredVisas {
		if v.Department == DeptInfoSec {
			hasInfoSec = true
		}
	}
	if !hasInfoSec {
		t.Errorf("Expected InfoSec visa when HasISAccessChange is true")
	}
}
func TestMatrixRow3_ISCommissioning(t *testing.T) {
	ctx := DocumentContext{
		Kind:            KindOrder,
		Title:           "Приказ о вводе в эксплуатацию автоматизированной системы",
		MatchedModuleID: "M-01",
	}
	res := CalculateVisas(ctx)
	hasLegal := false
	hasInfoSec := false
	hasIT := false
	for _, v := range res.RequiredVisas {
		if v.Department == DeptLegal {
			hasLegal = true
		}
		if v.Department == DeptInfoSec {
			hasInfoSec = true
		}
		if v.Department == DeptIT {
			hasIT = true
		}
	}
	if !hasLegal || !hasInfoSec || !hasIT {
		t.Errorf("Expected Legal, InfoSec and IT visas for IS commissioning, got: %+v", res.RequiredVisas)
	}
}
func TestMatrixRow6_WorkingGroup_SpecialITCondition(t *testing.T) {
	// Case A: Non-IT working group
	ctxA := DocumentContext{
		Kind:            KindDirective,
		Title:           "Распоряжение о создании рабочей группы по оптимизации архива",
		IsITProject:     false,
		MatchedModuleID: "M-07",
	}
	resA := CalculateVisas(ctxA)
	for _, v := range resA.RequiredVisas {
		if v.Department == DeptIT {
			t.Errorf("Did not expect IT visa for non-IT working group")
		}
		if v.Department == DeptLegal {
			t.Errorf("Did not expect Legal visa for working group (Row 6 specifies Legal=No)")
		}
	}
	// Case B: IT project working group
	ctxB := DocumentContext{
		Kind:            KindDirective,
		Title:           "Распоряжение о создании рабочей группы по внедрению ИИ",
		IsITProject:     true,
		MatchedModuleID: "M-07",
	}
	resB := CalculateVisas(ctxB)
	hasIT := false
	for _, v := range resB.RequiredVisas {
		if v.Department == DeptIT {
			hasIT = true
		}
	}
	if !hasIT {
		t.Errorf("Expected IT visa when IsITProject is true")
	}
}
func TestModulePrivilegeWarning(t *testing.T) {
	ctx := DocumentContext{
		Kind:            KindOrder,
		Title:           "Приказ о вводе в эксплуатацию информационной системы",
		MatchedModuleID: "M-01",
		CustomWording:   true,
	}
	res := CalculateVisas(ctx)
	if res.ModulePrivilege {
		t.Errorf("Expected ModulePrivilege to be false when CustomWording is true")
	}
	if res.PrivilegeWarning == "" {
		t.Errorf("Expected warning regarding loss of module privilege")
	}
}
