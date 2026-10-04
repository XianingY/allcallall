package models

import "time"

// AppVersionPolicy is the server's answer to "should this client keep running,
// or must it update first".
//
// Without it the only way to ship a fix is to hope users update: an app stuck on
// an old build keeps calling endpoints it no longer understands, and there is
// no way to tell it to stop. That is a version-control gap, not a feature gap -
// it matters most when a release contains a security fix.
//
// One row per platform. A missing row means "no policy", which is not the same
// as "no update required" and is reported distinctly so a client can tell the
// difference between an empty policy and an empty requirement.
type AppVersionPolicy struct {
	ID uint64 `gorm:"primaryKey;autoIncrement"`
	// Platform is "ios" or "android".
	Platform string `gorm:"size:16;not null;uniqueIndex"`
	// MinSupportedVersion is the oldest build allowed to keep using the service.
	// Anything below is refused.
	MinSupportedVersion string `gorm:"size:32;not null;default:'0.0.0'"`
	// LatestVersion is what users are told to move to. It does not gate
	// anything on its own; only MinSupportedVersion does.
	LatestVersion string `gorm:"size:32;not null;default:'0.0.0'"`
	// Message is shown on the blocking screen. Plain text; the client renders it.
	Message string `gorm:"size:512;not null;default:''"`
	// ForceUpdate distinguishes "you may continue but should update" from
	// "this build is no longer supported". A hard minimum is expressed with
	// MinSupportedVersion, so this flag exists for the softer case: a warning
	// the user can dismiss.
	ForceUpdate bool `gorm:"not null;default:false"`
	// ReleasedAt is when LatestVersion shipped, so a client can tell a fresh
	// release from a stale row that was never updated.
	ReleasedAt *time.Time
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (AppVersionPolicy) TableName() string { return "app_version_policies" }
