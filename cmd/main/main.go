package main

import (
	"fmt"
	"github.com/minio/minio/cmd"
	"github.com/minio/minio/internal/bucket/lifecycle"
	"github.com/minio/pkg/v2/policy"
	"github.com/minio/pkg/v2/policy/condition"
	xml "github.com/minio/xxml"
	"time"
	"unsafe"
)

func main() {
	bu := cmd.BucketMetadata{}

	fmt.Println(unsafe.Sizeof(bu))
	fmt.Println(unsafe.Sizeof(policy.BucketPolicy{
		Version: "12-10-2012",
		Statements: []policy.BPStatement{
			policy.NewBPStatement("",
				policy.Allow,
				policy.NewPrincipal("*"),
				policy.NewActionSet(policy.PutObjectAction),
				policy.NewResourceSet(policy.NewResource("mybucket/myobject*")),
				condition.NewFunctions(),
			),
		},
	}))
	policy := lifecycle.Lifecycle{
		Rules: []lifecycle.Rule{
			{
				ID:     "rule-1",
				Status: "Enabled",
				Transition: lifecycle.Transition{
					Days:         lifecycle.TransitionDays(3),
					StorageClass: "TIER-1",
				},
			},
		},
	}
	po, _ := xml.Marshal(policy)
	bu.LifecycleConfigXML = po
	fmt.Println("life", len(po), unsafe.Sizeof(Rule{
		ID:     "rule-1",
		Status: "Enabled",
		Expire: &Expiration{
			Days: 3,
			Date: time.Now(),
		},
		Filter: &Filter{
			Prefix: "prefix",
		},
	}))

	fmt.Println(unsafe.Sizeof(InventoryConfiguration{}))

}

type LifeCycle struct {
	XMLName xml.Name `xml:"LifecycleConfiguration"`
	Xmlns   string   `xml:"xmlns,attr"`
	Rules   []*Rule  `xml:"Rule,omitempty"`
}

type Rule struct {
	XMLName xml.Name    `xml:"Rule"`
	Expire  *Expiration `xml:"LocalExpiration"`
	Filter  *Filter     `xml:"Filter"`
	ID      string      `xml:"ID"`
	Status  string      `xml:"Status"`
}

type Expiration struct {
	XMLName xml.Name  `xml:"LocalExpiration"`
	Date    time.Time `xml:"Date,omitempty"`
	Days    int       `xml:"Days,omitempty"`
}

type InventoryConfiguration struct {
	XMLName                xml.Name       `xml:"InventoryConfiguration"`
	Destination            Destination    `xml:"Destination"`
	IsEnabled              bool           `xml:"IsEnabled"`
	Filter                 Filter         `xml:"Filter"`
	ID                     string         `xml:"Id"`
	IncludedObjectVersions string         `xml:"IncludedObjectVersions"`
	OptionalFields         OptionalFields `xml:"OptionalFields"`
	Schedule               Schedule       `xml:"Schedule"`
}

type Destination struct {
	S3BucketDestination S3BucketDestination `xml:"S3BucketDestination"`
}

type S3BucketDestination struct {
	AccountId  string     `xml:"AccountId"`
	Bucket     string     `xml:"Bucket"`
	Encryption Encryption `xml:"Encryption"`
	Format     string     `xml:"Format"`
	Prefix     string     `xml:"Prefix"`
}

type Encryption struct {
	SSEKMS SSEKMS `xml:"SSE-KMS"`
	SSES3  string `xml:"SSE-S3"`
}

type SSEKMS struct {
	KeyId string `xml:"KeyId"`
}

type Filter struct {
	Prefix string `xml:"Prefix"`
}

type OptionalFields struct {
	Field string `xml:"Field"`
}

type Schedule struct {
	Frequency string `xml:"Frequency"`
}
