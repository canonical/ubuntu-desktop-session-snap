#!/bin/sh

if [ -z "${SNAP_USER_COMMON:-}" ]; then
  echo "SNAP_USER_COMMON is unset; cannot configure content-snap screencast" >&2
  return 1
fi

resource_file="$SNAP/gnome/usr/share/gnome-shell/org.gnome.Shell.Screencast.src.gresource"
resource_tool="$SNAP/gnome/usr/bin/gresource"
data_dir="$SNAP_USER_COMMON/gnome-shell-screencast-${SNAP_REVISION:-current}"
resource_list="$data_dir/resources.list"
stamp_file="$data_dir/content-stamp"
lock_dir="$data_dir.lock"

if ! content_stamp=$(stat -c '%Y:%i:%s' "$resource_file"); then
  echo "Failed to inspect the content-snap screencast modules" >&2
  return 1
fi

content_assets_ready() {
  [ -f "$stamp_file" ] &&
    [ "$(cat "$stamp_file")" = "$content_stamp" ] &&
    [ -f "$resource_list" ] &&
    [ -f "$data_dir/entrypoint.js" ] &&
    [ -f "$data_dir/js/main.js" ] || return 1

  while IFS= read -r resource_path; do
    case "$resource_path" in
      /org/gnome/Shell/Screencast/js/*.js)
        relative_path=${resource_path#/org/gnome/Shell/Screencast/}
        [ -f "$data_dir/$relative_path" ] || return 1
        ;;
    esac
  done < "$resource_list"

  return 0
}

if ! content_assets_ready; then
  if mkdir "$lock_dir" 2>/dev/null; then
    if ! mkdir -p "$data_dir/js"; then
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to create content-snap screencast directory" >&2
      return 1
    fi

    resource_list_tmp="$resource_list.$$"
    if ! "$resource_tool" list "$resource_file" > "$resource_list_tmp"; then
      rm -f "$resource_list_tmp"
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to list content-snap screencast modules" >&2
      return 1
    fi

    while IFS= read -r resource_path; do
      case "$resource_path" in
        /org/gnome/Shell/Screencast/js/*.js)
          relative_path=${resource_path#/org/gnome/Shell/Screencast/}
          relative_dir=${relative_path%/*}
          module_path="$data_dir/$relative_path"
          module_tmp="$module_path.$$"
          if ! mkdir -p "$data_dir/$relative_dir"; then
            rm -f "$resource_list_tmp"
            rmdir "$lock_dir" 2>/dev/null
            echo "Failed to create content-snap screencast module directory" >&2
            return 1
          fi
          if ! "$resource_tool" extract "$resource_file" "$resource_path" > "$module_tmp"; then
            rm -f "$module_tmp" "$resource_list_tmp"
            rmdir "$lock_dir" 2>/dev/null
            echo "Failed to extract content-snap screencast module: $resource_path" >&2
            return 1
          fi
          if ! mv "$module_tmp" "$module_path"; then
            rm -f "$module_tmp" "$resource_list_tmp"
            rmdir "$lock_dir" 2>/dev/null
            echo "Failed to install content-snap screencast module: $resource_path" >&2
            return 1
          fi
          ;;
      esac
    done < "$resource_list_tmp"

    if ! mv "$resource_list_tmp" "$resource_list"; then
      rm -f "$resource_list_tmp"
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to install content-snap screencast module list" >&2
      return 1
    fi

    entrypoint_tmp="$data_dir/entrypoint.js.$$"
    if ! printf '%s\n' "import {main} from './js/main.js';" "await main();" > "$entrypoint_tmp" ||
      ! mv "$entrypoint_tmp" "$data_dir/entrypoint.js"; then
      rm -f "$entrypoint_tmp"
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to install content-snap screencast entry point" >&2
      return 1
    fi

    stamp_tmp="$stamp_file.$$"
    if ! printf '%s\n' "$content_stamp" > "$stamp_tmp" ||
      ! mv "$stamp_tmp" "$stamp_file"; then
      rm -f "$stamp_tmp"
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to record content-snap screencast version" >&2
      return 1
    fi
    if ! rmdir "$lock_dir" 2>/dev/null && [ -d "$lock_dir" ]; then
      echo "Failed to release content-snap screencast lock" >&2
      return 1
    fi
  else
    attempts=0
    while ! content_assets_ready; do
      attempts=$((attempts + 1))
      if [ "$attempts" -ge 30 ]; then
        echo "Timed out waiting for content-snap screencast setup" >&2
        return 1
      fi
      sleep 1
    done
  fi
fi

GNOME_SHELL_SCREENCAST_ENTRY="$data_dir/entrypoint.js"
export GNOME_SHELL_SCREENCAST_ENTRY
