package common

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/gofrs/flock"
	"github.com/hashicorp/go-getter"
	urlhelper "github.com/hashicorp/go-getter/helper/url"

	"github.com/hashicorp/packer/helper/multistep"
	"github.com/hashicorp/packer/packer"
)

// StepDownload downloads a remote file using the download client within
// this package. This step handles setting up the download configuration,
// progress reporting, interrupt handling, etc.
//
// Uses:
//   cache packer.Cache
//   ui    packer.Ui
type StepDownload struct {
	// The checksum and the type of the checksum for the download
	Checksum     string
	ChecksumType string

	// A short description of the type of download being done. Example:
	// "ISO" or "Guest Additions"
	Description string

	// The name of the key where the final path of the ISO will be put
	// into the state.
	ResultKey string

	// The path where the result should go, otherwise it goes to the
	// cache directory.
	TargetPath string

	// A list of URLs to attempt to download this thing.
	Url []string

	// Extension is the extension to force for the file that is downloaded.
	// Some systems require a certain extension. If this isn't set, the
	// extension on the URL is used. Otherwise, this will be forced
	// on the downloaded file for every URL.
	Extension string
}

func (s *StepDownload) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)

	ui.Say(fmt.Sprintf("Retrieving %s", s.Description))

	var errs []error
	for i := range s.Url {
		u, err := urlhelper.Parse(s.Url[i])
		if err != nil {
			errs = append(errs, fmt.Errorf("url parse: %s", err))
			continue // may be another url will work
		}
		if s.ChecksumType != "none" {
			// add checksum to url query params as go getter will checksum for us
			q := u.Query()
			q.Set("checksum", s.ChecksumType+":"+s.Checksum)
			u.RawQuery = q.Encode()
		}

		targetPath := s.TargetPath
		if targetPath == "" {
			// generate shasum of url+checksum
			// to download file in cache path
			shaSum := sha1.Sum([]byte(u.String()))
			targetPath = hex.EncodeToString(shaSum[:])
			if s.Extension != "" {
				targetPath += "." + s.Extension
			}
		}
		targetPath, err = packer.CachePath(targetPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("CachePath: %s", err))
			continue // may be another url will work
		}
		lockFile := targetPath + ".lock"

		log.Printf("Acquiring lock for: %s (%s)", u.String(), lockFile)
		lock := flock.New(lockFile)
		lock.Lock()
		defer lock.Unlock()

		ui.Say(fmt.Sprintf("Trying %s", u.String()))
		if err := getter.GetFile(targetPath, u.String(), getter.WithCheggaaaProgressBarV1(), getter.WithContext(ctx)); err != nil {
			errs = append(errs, err)
			continue // may be another url will work
		}

		ui.Say(fmt.Sprintf("%s => %s", u.String(), targetPath))
		state.Put(s.ResultKey, targetPath)
		return multistep.ActionContinue
	}

	state.Put("error", fmt.Errorf("Downloading file: %v", errs))
	return multistep.ActionHalt

}

func (s *StepDownload) Cleanup(multistep.StateBag) {}
