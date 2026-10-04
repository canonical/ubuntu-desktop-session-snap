#!/bin/sh

if [ -z "${SNAP_USER_COMMON:-}" ]; then
  echo "SNAP_USER_COMMON is unset; cannot configure content-snap image loaders" >&2
  return 1
fi

triplet=x86_64-linux-gnu
data_dir="$SNAP_USER_COMMON/desktop-content-data-${SNAP_REVISION:-current}"
data_share="$data_dir/share"
mime_dir="$data_share/mime"
glycin_source_dir="$SNAP/gnome/usr/share/glycin-loaders/2+/conf.d"
glycin_config_dir="$data_share/glycin-loaders/2+/conf.d"
glycin_loader_dir="$SNAP/gnome/usr/libexec/glycin-loaders/2+"
glycin_wrapper_dir="$data_dir/glycin-loaders/2+"
mime_packages="$SNAP/gnome/usr/share/mime/packages"
loader_module_dir="$SNAP/gnome/usr/lib/$triplet/gdk-pixbuf-2.0/2.10.0/loaders"
loader_query="$SNAP/gnome/usr/lib/$triplet/gdk-pixbuf-2.0/gdk-pixbuf-query-loaders"
loader_cache="$SNAP_USER_COMMON/gdk-pixbuf-loaders-${SNAP_REVISION:-current}.cache"
stamp_file="$data_dir/content-stamp"
lock_dir="$SNAP_USER_COMMON/desktop-content-data-${SNAP_REVISION:-current}.lock"

if [ -z "${LD_LIBRARY_PATH:-}" ]; then
  echo "LD_LIBRARY_PATH is unset; cannot configure content-snap Glycin loaders" >&2
  return 1
fi

if ! content_stamp=$(stat -c '%Y:%i' "$mime_packages" "$loader_module_dir" "$glycin_source_dir" "$glycin_loader_dir"); then
  echo "Failed to inspect content-snap image data" >&2
  return 1
fi

glycin_wrappers_ready() {
  found=0
  for glycin_loader in "$glycin_loader_dir"/*; do
    [ -f "$glycin_loader" ] && [ -x "$glycin_loader" ] || continue
    found=1
    [ -x "$glycin_wrapper_dir/$(basename "$glycin_loader")" ] || return 1
  done
  [ "$found" -eq 1 ]
}

content_assets_ready() {
  [ -f "$stamp_file" ] &&
    [ "$(cat "$stamp_file")" = "$content_stamp" ] &&
    [ -f "$mime_dir/mime.cache" ] &&
    [ -f "$glycin_config_dir/glycin-image-rs.conf" ] &&
    [ -f "$loader_cache" ] &&
    glycin_wrappers_ready
}

if ! content_assets_ready; then
  if mkdir "$lock_dir" 2>/dev/null; then
    if ! mkdir -p "$mime_dir" "$glycin_config_dir" "$glycin_wrapper_dir"; then
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to create content-snap image data directories" >&2
      return 1
    fi

    if [ "$(readlink "$mime_dir/packages")" = "$mime_packages" ]; then
      # Already linked (possibly left dangling by a concurrent invocation
      # while the content mount was still settling, or linked by a
      # concurrent run that won this lock first): nothing to do.
      :
    elif [ -e "$mime_dir/packages" ] || [ -L "$mime_dir/packages" ]; then
      if ! rm -f "$mime_dir/packages" ||
         ! ln -s "$mime_packages" "$mime_dir/packages"; then
        rmdir "$lock_dir" 2>/dev/null
        echo "Failed to update content-snap MIME package link" >&2
        return 1
      fi
    elif ! ln -s "$mime_packages" "$mime_dir/packages"; then
      # A concurrent invocation may have created the link between the
      # existence checks above and this ln; that is benign as long as it
      # points at this content snap's packages directory.
      if [ "$(readlink "$mime_dir/packages")" != "$mime_packages" ]; then
        rmdir "$lock_dir" 2>/dev/null
        echo "Failed to link content-snap MIME packages" >&2
        return 1
      fi
    fi

    export XDG_DATA_DIRS="$data_share:${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"
    if ! "$SNAP/gnome/usr/bin/update-mime-database" "$mime_dir"; then
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to generate content-snap MIME cache" >&2
      return 1
    fi

    for glycin_loader in "$glycin_loader_dir"/*; do
      [ -f "$glycin_loader" ] && [ -x "$glycin_loader" ] || continue
      glycin_wrapper="$glycin_wrapper_dir/$(basename "$glycin_loader")"
      glycin_wrapper_tmp="$glycin_wrapper.$$"
      if ! {
        printf '%s\n' '#!/bin/sh'
        printf "export LD_LIBRARY_PATH='%s'\n" "$LD_LIBRARY_PATH"
        printf "exec '%s' \"\$@\"\n" "$glycin_loader"
      } > "$glycin_wrapper_tmp" ||
        ! chmod 0755 "$glycin_wrapper_tmp" ||
        ! mv "$glycin_wrapper_tmp" "$glycin_wrapper"; then
        rm -f "$glycin_wrapper_tmp"
        rmdir "$lock_dir" 2>/dev/null
        echo "Failed to prepare a content-snap Glycin loader wrapper" >&2
        return 1
      fi
    done

    for glycin_config in "$glycin_source_dir"/*.conf; do
      glycin_tmp="$glycin_config_dir/.$(basename "$glycin_config").$$"
      if ! sed \
        -e "s#/usr/libexec/glycin-loaders/2+/#$glycin_wrapper_dir/#g" \
        "$glycin_config" > "$glycin_tmp"; then
        rm -f "$glycin_tmp"
        rmdir "$lock_dir" 2>/dev/null
        echo "Failed to prepare content-snap Glycin loader configuration" >&2
        return 1
      fi
      if ! mv "$glycin_tmp" "$glycin_config_dir/$(basename "$glycin_config")"; then
        rm -f "$glycin_tmp"
        rmdir "$lock_dir" 2>/dev/null
        echo "Failed to install content-snap Glycin loader configuration" >&2
        return 1
      fi
    done

    loader_tmp="$loader_cache.$$"
    if ! "$loader_query" "$loader_module_dir"/*.so > "$loader_tmp"; then
      rm -f "$loader_tmp"
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to generate content-snap GdkPixbuf loader cache" >&2
      return 1
    fi
    if ! mv "$loader_tmp" "$loader_cache"; then
      rm -f "$loader_tmp"
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to install content-snap GdkPixbuf loader cache" >&2
      return 1
    fi

    stamp_tmp="$stamp_file.$$"
    if ! printf '%s\n' "$content_stamp" > "$stamp_tmp" ||
      ! mv "$stamp_tmp" "$stamp_file"; then
      rm -f "$stamp_tmp"
      rmdir "$lock_dir" 2>/dev/null
      echo "Failed to record content-snap image data version" >&2
      return 1
    fi
    if ! rmdir "$lock_dir" 2>/dev/null && [ -d "$lock_dir" ]; then
      echo "Failed to release content-snap image data lock" >&2
      return 1
    fi
  else
    attempts=0
    while ! content_assets_ready; do
      attempts=$((attempts + 1))
      if [ "$attempts" -ge 30 ]; then
        echo "Timed out waiting for content-snap image data setup" >&2
        return 1
      fi
      sleep 1
    done
  fi
fi

export XDG_DATA_DIRS="$data_share:${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"
export GDK_PIXBUF_MODULE_FILE="$loader_cache"
