var dataHistory = {}
var currentRequest = null

jQuery.ajaxPrefilter(function(options, originalOptions, xhr) {
    if(options.spinner) {
        var spinner = jQuery(options.spinner);
        if(spinner && spinner.length > 0) {
            var timeoutId = setTimeout(function() { spinner.show(); }, 250);
            xhr.always(function() {
                clearTimeout(timeoutId);
                spinner.hide();
            });
        }
    }
});

$(document).ready(function() {
    // Load up our list of servers in the dropdown.
    $.ajax("/_getServerGroups", {
        success: function(data, statusCode, xhr) {
            if(data.success && data.payload.groups) {
                $.each(data.payload.groups, function(i, group) {
                    // Add in the category name and divider if this isn't the default category.
                    if(group.groupName != "") {
                        $("#serverList")
                        .append($("<option></option>").html(group.groupName).val("empty"))
                        .append($("<option></option>").html("--------------").val("empty"))
                    }

                    // Add in the servers now.
                    $.each(group.servers, function(i, server) {
                        $("#serverList").append($("<option></option>").html(server.displayName).val(server.internalName))
                    })

                    // Add in a spacer at the end.
                    $("#serverList").append($("<option></option>").html("").val("empty"))
                })

                // Remove the last empty option to tidy up the dropdown.
                $("#serverList option:last-child").remove()
            }
        }
    })

    $("#serverList").on('change', function(e) {
        e.preventDefault()

        // Short circuit if this isn't an actual server selection.
        var selection = $("#serverList option:selected")
        if(selection.val() == "empty") {
            return
        }

        // Clear out any existing errors.
        $("#errors").empty()

        // If we have an existing loading call, abort it.
        if(currentRequest != null) {
            currentRequest.abort()
        }

        var retryButton = $("<button></button>").addClass("btn").html("Reload")
            .on('click', function(e) { $("#serverList").change() })


        // Try and get a current process list snapshot for the selected server.
        currentRequest = $.ajax("/_getCurrentSnapshot/" + selection.val(), {
            success: function(data, statusCode, xhr) {
                if(data.success && data.events) {
                } else if(!data.success && data.errorMessage) {
                    var message = "We encountered an error while attempting to query the database.  Here's what the server said: <i>" + data.errorMessage + "</i>"
                    showErrorMessage("Error while querying server!", message, [retryButton])
                }

                currentRequest = null
            },
            error: function(xhr, textStatus, errorThrown) {
                if(textStatus == "timeout") {
                    var message = "We timed out trying to get the most recent process list from the database.  You can try and reload the process list or choose another database server.";
                    showErrorMessage("Timeout!", message, [retryButton])
                }

                currentRequuest = null
            },
            timeout: 6000,
            spinner: "#spinner"
        })
    })
})

function showErrorMessage(title, message, appends) {
    var alertBlock = $("<div></div>")
    alertBlock.addClass("alert alert-block alert-error fade in")
    alertBlock.html(
        '<h4>' + title + '</h4>' +
        '<p>' + message + '</p>'
    )

    // Append our appends if we have any.
    if(appends) {
        var appendBlock = $("<p></p>")
        $.each(appends, function(i, append) {
            appendBlock.append(append)
        })

        alertBlock.append(appendBlock)
    }

    // Clear any previous errors before showing this one.
    $("#errors").empty()
    $("#errors").append(alertBlock)
}
