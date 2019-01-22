Resource janitor is an application that can automate resource clean up for projects where resources are created quickly for testing and not always deleted despite the resource not being needed any more. Right now, this tool supports image and instance clean up.

This tool has support for the following flags:

`--project`: GCP project to run the tool against
`image-delimiter`: used to parse image names to try to somewhat smartly decide whether an image is part of a series or if it's a singleton that a human should look at deleting
`workers`: how many workers to use when issuing deletes to the API. Useful for speeding up deletion of a lot of resources
`older-than`: how old a resource needs to be before it's considered for deletion
`log-file`: fairly self-explanatory
`not-dry-run`: flag that must be present to actually start deleting resources. If this flag is not present, all commands will be treated as dry runs and only the decisions on whether a resource will be deleted or not (and reasoning why) will be logged.
