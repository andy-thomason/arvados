$(document).on('shown.bs.modal', '#add-group-modal', function(event) {
    $('input[type=text]', event.target).val('');
    $('#add-group-error', event.target).hide();
}).on('submit', '#add-group-form', function(event) {
    var $form = $(event.target),
    $submit = $(':submit', $form),
    $error = $('#add-group-error', $form),
    group_name = $('input[name="group_name"]', $form).val();

    $submit.prop('disabled', true);

    $error.hide();
    $.ajax('/groups',
           {method: 'POST',
            dataType: 'json',
            data: {group: {name: group_name, group_class: 'role'}},
            context: $form}).
        done(function(data, status, jqxhr) {
            location.reload();
        }).
        fail(function(jqxhr, status, error) {
            var errlist = jqxhr.responseJSON.errors;
            var errmsg;
            if (Array.isArray(errlist)) {
                errmsg = errlist.join();
            } else {
                errmsg = ("The server returned an error when creating " +
                          "this group (status " + jqxhr.status +
                          ": " + errlist + ").");
            }
            $error.text(errmsg);
            $error.show();
            $submit.prop('disabled', false);
        });
    return false;
});
