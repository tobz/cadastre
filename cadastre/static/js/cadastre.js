var maximumRecentDataAmount = 5
var currentRecentSelection = 0

var recentData = []
var currentRequest = null
var currentServer = null
var currentServerDisplayName = ""
var currentSnapshot = null
var viewFilters = {
    "runningFilter": function(snapshotEvent) {
        return $("#runningQueries").is(":checked") && (snapshotEvent.command.toLowerCase() != "sleep" && snapshotEvent.status.toLowerCase().indexOf("lock") == -1)
    },
    "lockedFilter": function(snapshotEvent) {
        return $("#lockedQueries").is(":checked") && (snapshotEvent.status.toLowerCase().indexOf("lock") !== -1)
    },
    "sleepingFilter": function(snapshotEvent) {
        return $("#sleepingQueries").is(":checked") && (snapshotEvent.command.toLowerCase() == "sleep")
    }
}

jQuery.ajaxPrefilter(function(options, originalOptions, xhr) {
    if(options.spinner) {
        var spinner = jQuery(options.spinner)
        var onSpinnerShow = options.onSpinnerShow
        if(spinner && spinner.length > 0) {
            var timeoutId = setTimeout(function() {
                if(onSpinnerShow && typeof(onSpinnerShow) == "function") {
                    onSpinnerShow()
                }
                spinner.show()
            }, 250)

            xhr.always(function() {
                clearTimeout(timeoutId)
                spinner.hide()
            })
        }
    }
})

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

        currentServerDisplayName = selection.html()

        // Clear out any existing event content since we're loading a brand new server.
        clearEventContent()

        // Get the latest data for the given server.
        pullLatestData(selection.val(), function(serverName, data) {
            // Set our currently server to this one so reclicks on the dropdown don't start a full content panel refresh.
            currentServer = serverName

            // Draw our events content.
            populateEvents(selection.html(), currentSnapshot, true)
        })
    })

    $(".view-filter").on('change', function(e) {
        // Trigger a redraw.
        redrawEvents()
    })
})

function eventMatchesViewState(snapshotEvent) {
    matches = false

    // Go through every configured view filter.
    for(var filterName in viewFilters) {
        // If the filter matches, it means this event belongs in the events display.
        var match = viewFilters[filterName]
        if(match(snapshotEvent)) {
            matches = true
        }
    }

    // If we got a match above, it means it passes the query state check, so now
    // we need to make sure it matches our selected database.
    if(matches) {
        var selectedDatabase = $("#databaseList").val()
        return selectedDatabase == "*" || selectedDatabase == snapshotEvent.database
    }

    return false
}

function pullLatestData(serverName, successCallback) {
    // If we have an existing loading call, abort it.
    if(currentRequest != null) {
        currentRequest.abort()
    }

    var retryButton = $("<button></button>")
        .addClass("btn")
        .html("Retry")
        .on("click", function(e) {
            currentServer = ""

            // Just retrigger a server selection.  We might be here on our first attempt so the
            // event view options aren't even drawn yet or any of that shit.
            $("#serverList").change()
        })

    // Invoke our request.
    currentRequest = $.ajax("/_getCurrentSnapshot/" + serverName, {
        success: function(data, statusCode, xhr) {
            if(data.success) {
                // Set our current snapshot.
                currentSnapshot = data.payload.events

                // Update the database list based on the latest snapshot.
                populateDatabaseList(currentSnapshot)

                // Add this to our list of recent data.
                addToRecentData(currentServerDisplayName, data.payload.events)

                // Call the user-supplied callback.
                successCallback(serverName)
            } else if(!data.success && data.errorMessage) {
                var message = "We encountered an error while attempting to query the database.  Here's what the server said: <i>" + data.errorMessage + "</i>"
                showErrorMessage("Error while querying server!", message, [retryButton])
            }

            currentRequest = null
        },
        error: function(xhr, textStatus, errorThrown) {
            // Clear out any event-related stuff since we're about to show an error.
            clearEventContent()

            var title = ""
            var message = ""

            if(textStatus == "timeout") {
                title = "Timeout!"
                message = "We timed out trying to get the most recent process list from the database.  You can try and reload the process list or choose another database server.";
            }

            if(textStatus == "error") {
                title = "Error!"
                message = "We encountered an error trying to get the most recent process list from the database.  This could be indicative of a web server error or internet problem."
            }

            showErrorMessage(title, message, [retryButton])

            currentRequest = null
        },
        timeout: 6000,
        spinner: "#spinner",
        onSpinnerShow: function() { clearEventContent(false) }
    })
}

function populateDatabaseList(events)
{
    var databases = {}

    for(var i = 0; i < events.length; i++) {
        // Mark this database as being present if it's not empty.
        if(events[i].database != "") {
            databases[events[i].database] = true
        }
    }

    // Set our list of databases.
    $('#databaseList').empty()
    $('#databaseList').append($('<option></option>').val('*').html('*'))

    for(var databaseName in databases) {
        $('#databaseList').append($('<option></option>').val(databaseName).html(databaseName))
    }
}

function populateEventViewOptions(realTime) {
    // Populate the events view options area.
    var realTimeOption = $('<a></a>')
        .attr("data-toggle", "tab")
        .attr("href", "#realTimeContainer")
        .html("Real Time")
        .on('click', function(e) {
            e.preventDefault()

            // Don't do anything if we're already toggled to the real-time view.
            if($(this).hasClass('active')) {
                return
            }

            // Add our button to reload the data.
            var reloadButton = $('<button></button>')
                .addClass('btn btn-primary btn-block btn-shiftdown')
                .html('Reload')
                .on('click', function(e) {
                    e.preventDefault()

                    // Trigger a simple refresh.
                    refreshServerData()
                })

            $('#viewSuboptions').empty()

            var realTimeContainer = $('<div></div>').append($("<div></div>")
                .addClass("pull-left span1")
                .append($("<span></span>").html("Actions: "))
                .append(reloadButton)
            )
            $("#viewSuboptions").append(realTimeContainer)
            $("#viewSuboptions").append($("<div></div>").attr("id", "historicalLinks"))

            redrawRecentDataList()
        })

    var historicalOption = $('<a></a>')
        .attr("data-toggle", "tab")
        .attr("href", "#historicalContainer")
        .html('Historical')
        .on('click', function(e) {
            e.preventDefault()

            // Clear out the event table.
            clearEventContent(false)
        })

    var navSideBar = $("<ul></ul>").addClass("nav nav-tabs")
        .append($("<li></li>").append(realTimeOption))
        .append($("<li></li>").append(historicalOption))

    var reloadButton = $('<button></button>')
        .addClass('btn btn-primary btn-block btn-shiftdown')
        .html('Reload')
        .on('click', function(e) {
            e.preventDefault()

            // Trigger a simple refresh.
            refreshServerData()
        })

    var realTimeContainer = $('<div></div>')
        .attr("id", "realTimeContainer")
        .addClass("tab-pane")

    realTimeContainer.append(
        $("<div></div>")
            .addClass("span1")
            .append($("<span></span>").html("Actions: "))
            .append(reloadButton))

    realTimeContainer.append(
        $("<div></div>")
            .attr("id", "historicalLinks")
            .addClass("span11"))

    var historicalContainer = $("<div></div>")
        .attr("id", "historicalContainer")
        .addClass("tab-pane")

    var navContent = $("<div></div>")
        .addClass("tab-content")
        .append(realTimeContainer)
        .append(historicalContainer)

    var navGroup = $('<div></div>').addClass('tabbable tabs-left')
    navGroup.append(navSideBar)
    navGroup.append(navContent)

    var eventViewOptions = $('<div></div>').addClass('row-fluid')
    eventViewOptions.append(navGroup)

    $('#eventViewOptions').empty()
    $('#eventViewOptions').append(eventViewOptions)

    // Set our real-time or historical button based on what we're loading.
    if(realTime) {
        realTimeOption.click()
    } else {
        historicalOption.click()
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

    hadMatchingRows = false

    for(var i = 0; i < events.length; i++) {
        // Make sure this event belongs in the current view based on state toggles (sleeping, locked, etc)
        if(!eventMatchesViewState(events[i]))
            continue

        hadMatchingRows = true

        var eventRow = $('<tr></tr>')
        eventRow.html(
            '<td>' + events[i].id + '</td>' +
            '<td>' + events[i].timeElapsed + '</td>' +
            '<td>' + events[i].host.substr(0, events[i].host.indexOf(':')) + '</td>' +
            '<td>' + events[i].user + '</td>' +
            '<td data-database="' + events[i].database + '">' + events[i].database + '</td>' +
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

    // See if our view state completely borked the list of visible events, and if so, let the user know
    // that they are seeing nothing because they've filtered it all out.
    if(!hadMatchingRows) {
        eventTableBody.append($("<tr></tr>")
            .append($("<td></td>")
                .attr("colspan", "10")
                .addClass("no-results")
                .html("There are no events matching the given filters.")))
    }

    eventTable.append(eventTableHeader)
    eventTable.append(eventTableBody)

    // Clear the old events table and put in our new one.
    $('#eventTable').empty()
    $('#eventTable').append(eventTable)
}

function redrawEvents() {
    // Simply repopulate the events table with the current snapshot.
    populateEventsTable(currentSnapshot)
}

function populateEvents(serverName, events, realTime) {
    // Populate the events view options area.
    populateEventViewOptions(realTime)

    // Populate our events table.
    populateEventsTable(events)

    // If we're in real-time mode, redraw our historical data.
    if(realTime) {
        redrawRecentDataList()
    }
}

function addToRecentData(serverName, data) {
    recentData.unshift({
        "serverName": serverName,
        "timestamp": moment().format('X'),
        "dateTime": moment().format('HH:mm:ss'),
        "events": data
    })

    // Get the "recent data" array down to the mximum size if we overflowed it.
    while(recentData.length > maximumRecentDataAmount) {
        recentData.pop()
    }
}

function redrawRecentDataList() {
    var historicalContainer = $("<div></div>")
        .attr("id", "historicalLinks")
        .addClass("pull-left span11")
    historicalContainer.append($("<span></span>").html("Recent Data: "))

    var listHolder = $("<ul></ul>").addClass("nav nav-pills")

    for(var i = 0; i < recentData.length; i++) {
        // If this is the currently selected recent datapoint, draw it as a span and not a link.
        var listItem = $("<li></li>")
        var listLink = $("<a></a>")
            .attr("href", "#")
            .attr("data-rel", i)
            .html(recentData[i]["serverName"] + " - " + recentData[i]["dateTime"] + "&nbsp;")
            .on("click", function(e) {
                e.preventDefault()

                dataRel = $(this).attr('data-rel')

                // Set this item to the active item.
                $("#historicalLinks li.active").removeClass("active")
                $(this).parent().addClass("active")

                // Set the current snapshot to the selected recent snapshot and
                // redraw the events table.
                currentSnapshot = recentData[dataRel]["events"]
                redrawEvents()
            })

        // If this is the first item in the list, select it.  If we're ever redrawing the list, it's because
        // something changed which means we added a new value, and the latest value is always show, and thus
        // it needs to be selected.
        if(i == 0) {
            listItem.addClass("active")
        }

        listItem.append(listLink)
        listHolder.append(listItem)
    }

    historicalContainer.append(listHolder)

    $("#historicalLinks").replaceWith(historicalContainer)
}

function refreshServerData() {
    // Clear out any errors, just to be safe.
    $("#errors").empty()

    // Get the latest data for the given server.
    pullLatestData(currentServer, function(serverName) {
        // Clear out the existing event table.
        clearEventContent(false)

        // Populate only the event table itself.
        populateEventsTable(currentSnapshot)

        // Redraw historical data since we got new stuff.
        redrawRecentDataList()
    })
}

function clearEventContent(clearViewOptions) {
    if(typeof(clearViewOptions) === "undefined") clearViewOptions = true

    if(clearViewOptions) {
        $('#eventViewOptions').empty()
    }

    $('#eventTable').empty()

    recentData = []
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
