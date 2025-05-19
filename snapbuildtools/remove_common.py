#!/usr/bin/env python3

""" Removes files that are already in base snaps or have been generated
    in a previous part. Useful to remove files added by stage-packages
    due to dependencies, but that aren't required because they are
    already available in core22, gnome-42-2204 or gtk-common-themes,
    or have already been built by a previous part. """

import os
import glob
import argparse
import fnmatch
try:
    import yaml
except:
    print("YAML module not found. Please, add 'python3-yaml' to the 'build-packages' list.")
    pass

parser = argparse.ArgumentParser(prog="remove_common", description="An utility to remove from snaps files that are already available in extensions")
parser.add_argument('extension', nargs='*', default=[])
parser.add_argument('-e', '--exclude', nargs='+', help="A list of files and directories to exclude from checking")
parser.add_argument('-m', '--map', nargs='+', default=[], help="A list of snap_name:path pairs")
parser.add_argument('-v', '--verbose', action='store_true', default=False, help="Show extra info")
parser.add_argument('-q', '--quiet', action='store_true', default=False, help="Don't show any message")
parser.add_argument('--snap-prefix', default=None, help="Set snaps folder (only for debugging/testing)")
args = parser.parse_args()

class Configuration:
    """ Contains all the desired configuration for removing common files """
    def __init__(self, *,
                 verbose = False,
                 quiet = False,
                 mappings = [],
                 excludes = None,
                 extensions = [],
                 snap_prefix = None):
        """ Initialized the class

        Parameters
        ----------
        verbose : bool, optional
            If the program must be more verbose and show extra messages, by default False
        quiet : bool, optional
            If the program must be quiet and show no messages, by default False
        mappings : list, optional
            A list of strings with the extension mapping, in the form
            'extension_snap_name:mapping_path_prefix'. This is useful when the contents of an
            extension aren't mapped directly to the corresponding relative path
            in our snap. An example is gtk-common-themes, whose their paths must
            be prefixed by 'usr/' to match the corresponding path in our snap. By default []
        excludes : list, optional
            A list of strings with rules to exclude files. These rules can contain
            wildcard characters like in a shell command line; thus * and ?. By default None
        extensions : list, optional
            _description_, by default []
        """
        # specific case for themed icons
        self._global_excludes = ['usr/share/icons/*/index.theme']
        self._global_maps = ['gtk-common-themes:usr']

        if ( excludes is not None) and len(excludes) != 0:
            self._global_excludes += excludes

        # this is useful for tests, to change where the folders are
        self._snap_prefix = '/snap' if snap_prefix is None else snap_prefix
        self._custom_extensions = []
        self.verbose = verbose
        self.quiet = quiet
        self._mapping_list = mappings
        self._extensions_list = extensions
        self._extensions_paths = None
        self._yaml = None

        if verbose:
            print(f"Removing duplicates already in {extensions}")


    def add_extension_path(self, extension, files_path, path_mapping = None):
        """ Adds the path of an extension

        Parameters
        ----------
        extension : string
            The extension name, or None if it is the stage folder
        files_path : string
            The path where the data of the extension is stored
        path_mapping : string or None, optional
            The mapping path required to match the paths inside this
            extension snap and the paths in our snap. By default None
        """
        if (files_path is None) or (files_path == ""):
            return
        self._custom_extensions.append((extension, files_path, path_mapping))


    def add_exclude(self, exclude):
        self._global_excludes.append(exclude)


    @property
    def exclude_list(self):
        return self._global_excludes


    @property
    def extensions_paths(self):
        extensions = self._get_extensions_list(self._extensions_list)
        if len(extensions) == 0:
            raise RuntimeError("Called remove_common.py without a list of snaps, and no 'build-snaps' entry in the snapcraft.yaml file.")
        mappings = self._generate_mappings(self._global_maps, self._mapping_list)
        extensions_paths = self._generate_extensions_paths(extensions, mappings)
        return extensions_paths + self._custom_extensions


    def _load_snapcraft_yaml(self):
        """ Loads the snapcraft.yaml file in memory """

        # Don't load it if it was already loaded
        if self._yaml is not None:
            return

        if 'CRAFT_PROJECT_DIR' not in os.environ:
            return

        project_folder = os.environ['CRAFT_PROJECT_DIR']
        snapcraft_file_path = os.path.join(project_folder, "snapcraft.yaml")
        if not os.path.exists(snapcraft_file_path):
            snapcraft_file_path = os.path.join(project_folder, "snap", "snapcraft.yaml")
            if not os.path.exists(snapcraft_file_path):
                return
        with open(snapcraft_file_path, "r") as snapcraft_stream:
            self._yaml = yaml.load(snapcraft_stream, Loader=yaml.Loader)


    def _get_extensions_list(self, cmdline_extensions):
        """Returns an array with the extensions for this project.

        It returns either the list of extensions passed in the command line, or
        the list of extensions extracted from the snapcraft.yaml file.

        Parameters
        ----------
        cmdline_extensions : array of strings
            an array with the list of extensions passed by the command line.
            If no extensions were passed, it must be a zero-length array.

        Returns
        -------
        array of strings
            an array with the list of extensions desired. If no extensions are
            defined, it will return a zero-length array.

        Raises
        ------
        FileNotFoundError
            If the snapcraft.yaml file can't be found.
        """

        if len(cmdline_extensions) != 0:
            return cmdline_extensions

        self._load_snapcraft_yaml()
        parts_data = self._yaml["parts"]
        extensions = []
        for part_name in parts_data:
            part_data = parts_data[part_name]
            if "build-snaps" not in part_data:
                continue
            for extension in part_data["build-snaps"]:
                if extension not in extensions:
                    extensions.append(extension)
        return extensions


    def _generate_mappings(self, predefined_mappings, cmdline_mappings = []):
        """Parses the specific mappings for each snap

        It receives both the predefined mappings and the mappings passed by
        command line, and returns a dictionary with each snap and its corresponding
        mapping path.

        Parameters
        ----------
        predefined_mappings : array of strings
            An array of strings, each one in the format snap_name:mapping. Used
            for the "known mappings", like the one for gtk-common-themes.
        cmdline_mappings : array of strings
            An array of strings as received from argparse with the snap_name:mapping
            pairs.

        Returns
        -------
        dictionary
            A dictionary where each key is a string with a snap name and the
            value is another string with the mapping path for that snap, ended
            in '/'.

        Raises
        ------
        SyntaxError
            If the format of any of the mapping strings is not valid
        """

        mappings = {}

        for map in predefined_mappings + cmdline_mappings:
            elements = map.split(":")
            if len(elements) != 2:
                raise SyntaxError("Error in mapping. It must be in the format snap_name:path", {'filename': 'remove_common.py', 'text': map, 'lineno': 0, 'offset': 0})
            if elements[1] == '/':
                raise SyntaxError("The mapping can't be '/'", {'filename': 'remove_common.py', 'text': map, 'lineno': 0, 'offset': 0})
            while elements[1][0] == '/':
                elements[1] = elements[1][1:]
            if elements[1][-1] != '/':
                elements[1] += '/'
            mappings[elements[0]] = elements[1]

        return mappings


    def _generate_extensions_paths(self, extensions, mappings):
        """Generates the list of extensions paths from the list of extensions

        This list has the paths of each extension used, to know where to search
        for duplicates. Also, it contains the corresponding map for each path.

        Parameters
        ----------
        extensions : array of strings
            An array of strings with the names of the extensions used in this snap.
        mappings : dictionary
            A dictionary where each key is a string with a snap name and the
            value is another string with the mapping path for that snap, ended
            in '/'.

        Returns
        -------
        array of tuples
            An array of tuples, where the first element of each tuple is a string
            to the contents of each extension, and the second element is the mapping
            for that path, or None if no mapping is needed.
        """

        folders = []
        for snap in extensions:
            path = os.path.join(self._snap_prefix, snap, "current")
            map_path = mappings[snap] if snap in mappings else None
            folders.append((snap, path, map_path))
        return folders


def check_if_exists(config, relative_file_path):
    """Checks if an specific file does exist in any of the base paths.

    Checks if the specified file at `relative_file_path` does exist in
    any of the paths included in the `extensions_paths` array, taking into
    account the mapping specified.

    Mapping is paramount for some extension snaps like gtk-common-themes.
    In this specific case, the snap contains the `share` folder directly
    at its root folder, while any other snap would have it inside the
    `usr` folder. Thus, for that extension snap, the map must be `usr`
    to instruct this function to remove `usr` from the beginning of the
    relative path when checking for a file inside gtk-common-themes.

    Parameters
    ----------
    config : Configuration
        This object must contain the current configuration data
    relative_file_path : string
        The file path to search in the folder list, relative to the
        snap root.

    Returns
    -------
    boolean
        It returns True if the file does exist in, at least, one of the
        specified snaps, and false if it doesn't exist in any of them.
    """

    # Checks if an specific file does exist in any of the base paths
    for _, folder, map_path in config.extensions_paths:
        if (map_path is not None) and relative_file_path.startswith(map_path):
            relative_file_path2 = relative_file_path[len(map_path):]
            if relative_file_path2[0] == '/':
                relative_file_path2 = relative_file_path2[1:]
        else:
            relative_file_path2 = relative_file_path
        check_path = os.path.join(folder, relative_file_path2)
        if os.path.exists(check_path):
            if config.verbose:
                print(f"The path {relative_file_path} has been found inside {folder} with map {map_path}: {relative_file_path2}")
            return True
    return False

def find_symlinks(snap_folder):
    """"Find symlinks

    Searches the snap being built for symlinks, and stores the original file.
    This is needed to ensure that no dangling symlinks are left in the new
    snap.

    Parameters
    ----------
    snap_folder : string
        The path of the folder where are the possible symlinks to duplicated files.

    Returns
    -------
    dictionary string : array[string]
        A dictionary where each key is a destination file, and the value is an array
        with all the symlinks pointing to that file.
    """

    symlinks = {}
    for full_symlink_path in glob.glob(os.path.join(snap_folder, "**/*"), recursive=True):
        if not os.path.islink(full_symlink_path):
            continue
        destination = os.path.realpath(full_symlink_path)
        if not os.path.isfile(destination):
            continue
        if destination not in symlinks:
            symlinks[destination] = []
        symlinks[destination].append(full_symlink_path)
    return symlinks

def _remove_duplicated_file(full_file_path, symlink_list):
    """ Deletes a file that we know that is duplicated, and ensures that if a
    symlink points to it, the symlink will be replaced with a copy of the file.
    But if it is a symlink, it will be removed from the list of symlinks to
    avoid re-creating it in case that the symlink is deleted after.

    Parameters
    ----------
    full_file_path : string
        The full path to the file/symlink to delete

    symlink_list : dictionary string : [string]
        A dictionary with full path to files as keys, and an array of all the
        full paths of symlinks pointing to that file as the value.

    Returns
    -------
    An integer with the number of bytes freed after deleting that file.
    """

    if full_file_path in symlink_list:
        for link_path in symlink_list[full_file_path]:
            if os.path.exists(link_path):
                os.remove(link_path)
                os.link(full_file_path, link_path)
    duplicated_bytes = 0
    real_path = os.path.realpath(full_file_path)
    if os.path.exists(real_path):
        file_data = os.stat(full_file_path)
        if os.path.isfile(full_file_path) and (file_data.st_nlink == 1):
            duplicated_bytes = file_data.st_size
    os.remove(full_file_path)
    return duplicated_bytes

def main(*, snap_folder, config):
    """Main function

    Searches each file in 'snap_folder' inside each path in 'extensions_paths'
    to check if it is already available there, deleting it in that case, unless
    it matches any of the rules in 'exclude_list'.

    The check is done based only on the relative path and file name relative to
    'snap_folder', searching that inside each path in 'extensions_paths', although
    taking into account the mapping.

    Parameters
    ----------
    snap_folder : string
        The path of the folder where the staged .deb have been uncompressed (usually
        CRAFT_PART_INSTALL)
    config : Configuration
        A Configuration object with the desired configuration for extensions_paths
        and exclude_list.
    """

    duplicated_bytes = 0
    symlink_list = find_symlinks(snap_folder)

    for full_file_path in glob.glob(os.path.join(snap_folder, "**/*"), recursive=True):
        if not os.path.isfile(full_file_path) and not os.path.islink(full_file_path):
            continue
        relative_file_path = full_file_path[len(snap_folder):]
        if relative_file_path[0] == '/':
            relative_file_path = relative_file_path[1:]
        do_exclude = False
        for exclude in config.exclude_list:
            if fnmatch.fnmatch(relative_file_path, exclude):
                if config.verbose:
                    print(f"Excluding {relative_file_path} with rule {exclude}")
                do_exclude = True
                break
        if do_exclude:
            continue
        if check_if_exists(config, relative_file_path):
            duplicated_bytes += _remove_duplicated_file(full_file_path, symlink_list)
            if config.verbose:
                print(f"Removing duplicated file {relative_file_path} {full_file_path}")
    if not config.quiet:
        print(f"Removed {duplicated_bytes} bytes in duplicated files")


if __name__ == "__main__":
    configuration = Configuration(verbose=args.verbose,
                                  quiet=args.quiet,
                                  mappings=args.map,
                                  excludes=args.exclude,
                                  extensions=args.extension,
                                  snap_prefix=args.snap_prefix)
    if "CRAFT_STAGE" in os.environ:
        configuration.add_extension_path(None, os.environ["CRAFT_STAGE"], None)

    # This is the folder where to check for duplicates that are already
    # in other snaps, or in the stage because they were built in other
    # parts.
    snap_folder = os.environ["CRAFT_PART_INSTALL"]

    main(snap_folder=snap_folder, config = configuration)
