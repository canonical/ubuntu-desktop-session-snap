#!/bin/sh

if [ -z "${SNAP_USER_COMMON:-}" ]; then
  echo "SNAP_USER_COMMON is unset; cannot configure content-snap fonts" >&2
  return 1
fi

fontconfig_file="$SNAP_USER_COMMON/fontconfig-${SNAP_REVISION:-current}.conf"
if [ ! -f "$fontconfig_file" ]; then
  fontconfig_tmp="$fontconfig_file.$$"
  if ! sed \
    -e "s#<dir>/usr/share/fonts</dir>#<dir>$SNAP/gnome/usr/share/fonts</dir>#" \
    -e "s#<dir>/usr/local/share/fonts</dir>#<dir>$SNAP/gnome/usr/local/share/fonts</dir>#" \
    -e "s#<include ignore_missing=\"yes\">conf.d</include>#<include ignore_missing=\"yes\">$SNAP/gnome/etc/fonts/conf.d</include>#" \
    "$SNAP/gnome/etc/fonts/fonts.conf" > "$fontconfig_tmp"; then
    rm -f "$fontconfig_tmp"
    echo "Failed to generate content-snap fontconfig" >&2
    return 1
  fi
  if ! mv "$fontconfig_tmp" "$fontconfig_file"; then
    rm -f "$fontconfig_tmp"
    echo "Failed to install content-snap fontconfig" >&2
    return 1
  fi
fi

export FONTCONFIG_FILE="$fontconfig_file"
export FONTCONFIG_PATH="$SNAP/gnome/etc/fonts"
