// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2015-2020 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package daemon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/snapcore/snapd/client/clientutil"
	"github.com/snapcore/snapd/osutil/user"
	"github.com/snapcore/snapd/overlord/auth"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/overlord/swfeats"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/strutil"
)

var (
	appsCmd = &Command{
		Path:        "/v2/apps",
		GET:         getAppsInfo,
		POST:        postApps,
		Actions:     []string{"start", "stop", "restart"},
		ReadAccess:  interfaceOpenAccess{Interfaces: []string{"ros-snapd-support"}},
		WriteAccess: interfaceAuthenticatedAccess{Interfaces: []string{"ros-snapd-support"}, Polkit: polkitActionManage},
	}

	logsCmd = &Command{
		Path:       "/v2/logs",
		GET:        getLogs,
		ReadAccess: authenticatedAccess{Polkit: polkitActionManage},
	}
)

var serviceControlChangeKind = swfeats.RegisterChangeKind("service-control")

var newStatusDecorator = func(ctx context.Context, isGlobal bool, uid string) clientutil.StatusDecorator {
	if isGlobal {
		return nil
	} else {
		return nil
	}
}

func readMaybeBoolValue(query url.Values, name string) (bool, error) {
	if sel := query.Get(name); sel != "" {
		if v, err := strconv.ParseBool(sel); err != nil {
			return false, fmt.Errorf("invalid %s parameter: %q", name, sel)
		} else {
			return v, nil
		}
	}
	return false, nil
}

func getAppsInfo(c *Command, r *http.Request, user *auth.UserState) Response {
	query := r.URL.Query()
	opts := appInfoOptions{}
	switch sel := query.Get("select"); sel {
	case "":
		// nothing to do
	case "service":
		opts.service = true
	default:
		return BadRequest("invalid select parameter: %q", sel)
	}

	global, err := readMaybeBoolValue(query, "global")
	if err != nil {
		return BadRequest(err.Error())
	}

	appInfos, rspe := appInfosFor(c.d.overlord.State(), strutil.CommaSeparatedList(query.Get("names")), opts)
	if rspe != nil {
		return rspe
	}

	u, err := systemUserFromRequest(r)
	if err != nil {
		return BadRequest("cannot retrieve services: %v", err)
	}

	// For the root user, default to global to preserve the normal
	// behaviour and we make sure to match behaviour for snapctl.
	if u.Uid == "0" {
		global = true
	}

	sd := newStatusDecorator(r.Context(), global, u.Uid)
	clientAppInfos, err := clientutil.ClientAppInfosFromSnapAppInfos(appInfos, sd)
	if err != nil {
		return InternalError("%v", err)
	}

	return SyncResponse(clientAppInfos)
}

type appInfoOptions struct {
	service bool
}

func (opts appInfoOptions) String() string {
	if opts.service {
		return "service"
	}

	return "app"
}

// appInfosFor returns a sorted list apps described by names.
//
//   - If names is empty, returns all apps of the wanted kinds (which
//     could be an empty list).
//   - An element of names can be a snap name, in which case all apps
//     from the snap of the wanted kind are included in the result (and
//     it's an error if the snap has no apps of the wanted kind).
//   - An element of names can instead be snap.app, in which case that app is
//     included in the result (and it's an error if the snap and app don't
//     both exist, or if the app is not a wanted kind)
//
// On error an appropriate *apiError is returned; a nil *apiError means
// no error.
//
// It's a programming error to call this with wanted having neither
// services nor commands set.
func appInfosFor(st *state.State, names []string, opts appInfoOptions) ([]*snap.AppInfo, *apiError) {
	snapNames := make(map[string]bool)
	requested := make(map[string]bool)
	for _, name := range names {
		requested[name] = true
		name, _ = splitAppName(name)
		snapNames[name] = true
	}

	snaps, err := allLocalSnapInfos(st, snapSelectNone, snapNames)
	if err != nil {
		return nil, InternalError("cannot list local snaps! %v", err)
	}

	found := make(map[string]bool)
	appInfos := make([]*snap.AppInfo, 0, len(requested))
	for _, snp := range snaps {
		snapName := snp.info.InstanceName()
		apps := make([]*snap.AppInfo, 0, len(snp.info.Apps))
		for _, app := range snp.info.Apps {
			if !opts.service || app.IsService() {
				apps = append(apps, app)
			}
		}

		if len(apps) == 0 && requested[snapName] {
			return nil, AppNotFound("snap %q has no %ss", snapName, opts)
		}

		includeAll := len(requested) == 0 || requested[snapName]
		if includeAll {
			// want all services in a snap
			found[snapName] = true
		}

		for _, app := range apps {
			appName := snapName + "." + app.Name
			if includeAll || requested[appName] {
				appInfos = append(appInfos, app)
				found[appName] = true
			}
		}
	}

	for k := range requested {
		if !found[k] {
			if snapNames[k] {
				return nil, SnapNotFound(k, fmt.Errorf("snap %q not found", k))
			} else {
				snap, app := splitAppName(k)
				return nil, AppNotFound("snap %q has no %s %q", snap, opts, app)
			}
		}
	}

	sort.Sort(snap.AppInfoBySnapApp(appInfos))

	return appInfos, nil
}

// this differs from snap.SplitSnapApp in the handling of the
// snap-only case:
//
//	snap.SplitSnapApp("foo") is ("foo", "foo"),
//	splitAppName("foo") is ("foo", "").
func splitAppName(s string) (snap, app string) {
	if idx := strings.IndexByte(s, '.'); idx > -1 {
		return s[:idx], s[idx+1:]
	}

	return s, ""
}

func getLogs(c *Command, r *http.Request, user *auth.UserState) Response {
	return InternalError("not supported in this build")
}




func decodeServiceInstruction(body io.ReadCloser, u *user.User) (interface{}, error) {
	return nil, fmt.Errorf("service instructions not supported")
}

var systemUserFromRequest = func(r *http.Request) (*user.User, error) {
	uid, err := uidFromRequest(r)
	if err != nil {
		return nil, err
	}

	u, err := user.LookupId(strconv.Itoa(int(uid)))
	if err != nil {
		return nil, err
	}

	// ensure that we only get the root user on a uid == 0 input
	if u.Uid == "0" && uid != 0 {
		return nil, fmt.Errorf("unknown uid: %d", uid)
	}
	return u, nil
}

func postApps(c *Command, r *http.Request, user *auth.UserState) Response {
	return BadRequest("service operations not supported in this build")
}


func namesToSnapNames(inst interface{}) []string {
	return nil
}
func (e *serviceActionConflictError) Error() string { return "conflicting service action" }

func doServiceControl(st interface{}, appInfos interface{}, inst interface{}, u interface{}) (interface{}, error) {
	return nil, fmt.Errorf("service control not supported in this build")
}

type serviceActionConflictError struct{}
