package render

import (
	"fmt"
	"strings"

	"github.com/derailed/k9s/internal/client"
	"github.com/gdamore/tcell"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Alias renders a aliases to screen.
type Alias struct{}

// ColorerFunc colors a resource row.
func (Alias) ColorerFunc() ColorerFunc {
	return func(ns string, re RowEvent) tcell.Color {
		return tcell.ColorMediumSpringGreen
	}
}

// Header returns a header row.
func (Alias) Header(ns string) HeaderRow {
	return HeaderRow{
		Header{Name: "RESOURCE"},
		Header{Name: "COMMAND"},
		Header{Name: "APIGROUP"},
	}
}

// Render renders a K8s resource to screen.
// BOZO!! Pass in a row with pre-alloc fields??
func (Alias) Render(o interface{}, gvr string, r *Row) error {
	a, ok := o.(AliasRes)
	if !ok {
		return fmt.Errorf("expected AliasRes, but got %T", o)
	}
	_ = a

	r.ID = gvr
	gvr1 := client.GVR(a.GVR)
	grp, res := gvr1.ToRAndG()
	r.Fields = append(r.Fields,
		res,
		strings.Join(a.Aliases, ","),
		grp,
	)

	return nil
}

// Helpers...

// AliasRes represents an alias resource.
type AliasRes struct {
	GVR     string
	Aliases []string
}

// GetObjectKind returns a schema object.
func (AliasRes) GetObjectKind() schema.ObjectKind {
	return nil
}

// DeepCopyObject returns a container copy.
func (a AliasRes) DeepCopyObject() runtime.Object {
	return a
}
