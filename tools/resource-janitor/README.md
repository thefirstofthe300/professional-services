Resource janitor is an application that can automate resource clean up for projects where resources are created quickly for testing and not always deleted despite the resource not being needed any more. Right now, this tool supports image and instance clean up.

This tool has support for the following flags:

* `--project`: string: GCP project to run the tool against
* `--image-delimiter`: string: used to parse image names to try to somewhat smartly decide whether an image is part of a series or if it's a singleton that a human should look at deleting
* `--workers`: int: how many workers to use when issuing deletes to the API. Useful for speeding up deletion of a lot of resources
* `--older-than`: int: how old a resource needs to be in seconds before it's considered for deletion
* `--log-file`: string: path to log file
* `--not-dry-run`: bool: flag that must be present to actually start deleting resources. If this flag is not present, all commands will be treated as dry runs and only the decisions on whether a resource will be deleted or not (and reasoning why) will be logged.
* `--delete-singletons`: bool: if set, the flag
* `--blacklist-file`: string: path to a yaml file that contains a list of instances and images to be ignored. Note that the patterns in the file should use the [Golang regex syntax](https://github.com/google/re2/wiki/Syntax). See `sample-blacklist.yaml` for an example.
