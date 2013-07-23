var dataHistory = {}
var currentRequest = null
var currentServer = null

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

        // If this is the current server, just initiate a refresh instead of a full content panel refresh.
        if(currentServer == selection.val()) {
            refreshServerData()
            return
        }

        // Clear out any existing event content since we're loading a brand new server.
        clearEventContent()

        // Get the latest data for the given server.
        pullLatestData(selection.val(), function(serverName, data) {
            // Set our currently server to this one so reclicks on the dropdown don't start a full content panel refresh.
            currentServer = serverName

            populateEvents(selection.html(), data.events, true)
        })
    })
})

function pullLatestData(serverName, successCallback) {
    // If we have an existing loading call, abort it.
    if(currentRequest != null) {
        currentRequest.abort()
    }

    // Invoke our request.
    currentRequest = $.ajax("/_getCurrentSnapshot/" + serverName, {
        success: function(data, statusCode, xhr) {
            if(data.success) {
                successCallback(serverName, data.payload)
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

            currentRequest = null
        },
        timeout: 6000,
        spinner: "#spinner"
    })
}

function populateEventsHeader(serverName) {
    // Build our table header and real-time/historical button group.
    var header = $('<div></div>')
    var headerTitle = $('<h3></h3>').html(serverName)
    var headerTime = $('<small></small>').attr('id', 'displayTime')
    headerTitle.append(headerTime)
    header.append(headerTitle)

    $('#eventsHeader').empty()
    $('#eventsHeader').append(header)

    updateEventsHeaderTime()
}

function updateEventsHeaderTime() {
    $('#displayTime').html('&nbsp;' + moment().format('MMMM Do YYYY, HH:mm:ss'))
}

function populateEventViewOptions(realTime) {
    // Populate the events view options area.
    var buttonGroup = $('<div></div>').addClass('btn-group btn-group-vertical span1').attr('data-toggle', 'buttons-radio')
    var realTimeButton = $('<button></button>').addClass('btn btn-primary btn-block').html('Real Time').on('click', function(e) {
        e.preventDefault()

        // Reset any historical data.
        historicalData = {}

        // Add our button to reload the data.
        var reloadButton = $('<button></button>').addClass('btn btn-primary').html('Reload').on('click', function(e) {
            e.preventDefault()

            // Trigger a simple refresh.
            refreshServerData()
        })

        $('#viewSuboptions').empty()
        $('#viewSuboptions').append(reloadButton)
    })
    var historicalButton = $('<button></button>').addClass('btn btn-primary btn-block').html('Historical').on('click', function(e) {
        e.preventDefault()
    })

    buttonGroup.append(realTimeButton).append(historicalButton)

    var eventViewOptions = $('<div></div>').addClass('row-fluid')
    eventViewOptions.append(buttonGroup)
    eventViewOptions.append(
        $('<div></div>').addClass('span11 well').attr('id', 'viewSuboptions').html('Do stuff here.')
    )

    $('#eventViewOptions').empty()
    $('#eventViewOptions').append(eventViewOptions)

    // Set our real-time or historical button based on what we're loading.
    if(realTime) {
        realTimeButton.click()
    } else {
        historicalButton.click()
    }
}

function populateEventsTable(events) {
    // Build our table.
    var eventTable = $('<table></table>').addClass('table').attr('id', 'eventTable')
    var eventTableBody = $('<tbody></tbody>')

    var eventTableHeader = $('<thead></thead>')
    eventTableHeader.html(
        '<tr>' +
        '<td style="width: 1%">ID</td>' +
        '<td style="width: 1%">Time</td>' +
        '<td style="width: 5%">Host</td>' +
        '<td style="width: 5%">User</td>' +
        '<td style="width: 5%">Database</td>' +
        '<td style="width: 20%">Status</td>' +
        '<td style="width: 60%">SQL</td>' +
        '<td style="width: 1%">Rows Sent</td>' +
        '<td style="width: 1%">Rows Examined</td>' +
        '<td style="width: 1%">Rows Read</td>' +
        '</tr>'
    )

    for(var i = 0; i < events.length; i++) {
        var eventRow = $('<tr></tr>')
        eventRow.html(
            '<td>' + events[i].id + '</td>' +
            '<td>' + events[i].timeElapsed + '</td>' +
            '<td>' + events[i].host.substr(0, events[i].host.indexOf(':')) + '</td>' +
            '<td>' + events[i].user + '</td>' +
            '<td>' + events[i].database + '</td>' +
            '<td>' + events[i].status + '</td>' +
            '<td>' + events[i].sql + '</td>' +
            '<td>' + events[i].rowsSent + '</td>' +
            '<td>' + events[i].rowsExamined + '</td>' +
            '<td>' + events[i].rowsRead + '</td>'
        )

        // Assign the background color to the row based on the query status.
        if(events[i].status.toLowerCase().indexOf('lock') !== -1) {
            eventRow.addClass('query-locked')
        } else if (events[i].command == "Sleep") {
            eventRow.addClass('query-sleeping')
        } else {
            eventRow.addClass('query-normal')
        }

        eventTableBody.append(eventRow)
    }

    eventTable.append(eventTableHeader)
    eventTable.append(eventTableBody)

    $('#eventTable').empty()
    $('#eventTable').append(eventTable)
}

function populateEvents(serverName, events, realTime) {
    // Build our events header - server name, time, etc.
    populateEventsHeader(serverName)

    // Populate the events view options area.
    populateEventViewOptions(realTime)

    // Populate our events table.
    populateEventsTable(events)
}

function refreshServerData() {
    // Clear out any errors, just to be safe.
    $("#errors").empty()

    // Clear out the existing event table.
    $('#eventTable').empty()

    // Get the latest data for the given server.
    pullLatestData(currentServer, function(serverName, data) {
        // Update the time of the snapshot.
        updateEventsHeaderTime()

        // Populate only the event table itself.
        populateEventsTable(data.events)
    })
}

function clearEventContent() {
    $('#eventsHeader').empty()
    $('#eventViewOptions').empty()
    $('#eventTable').empty()
}

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
