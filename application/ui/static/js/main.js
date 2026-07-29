// showConfirmDialog/showAlertDialog wrap the shared <dialog> elements in
// base.html, standing in for the browser's native confirm()/alert() (which
// look and behave inconsistently across browsers and can't be styled) with
// something that matches the rest of the site. Both resolve once the user
// responds — showConfirmDialog resolves to true/false, showAlertDialog just
// resolves once dismissed.
function showConfirmDialog(message) {
    return new Promise(function (resolve) {
        var dialog = document.getElementById('confirm-dialog');
        document.getElementById('confirm-dialog-message').textContent = message;
        var confirmBtn = document.getElementById('confirm-dialog-confirm');
        var cancelBtn = document.getElementById('confirm-dialog-cancel');

        function cleanup(result) {
            confirmBtn.removeEventListener('click', onConfirm);
            cancelBtn.removeEventListener('click', onCancel);
            dialog.removeEventListener('cancel', onCancel);
            dialog.close();
            resolve(result);
        }
        function onConfirm() { cleanup(true); }
        function onCancel() { cleanup(false); }

        confirmBtn.addEventListener('click', onConfirm);
        cancelBtn.addEventListener('click', onCancel);
        dialog.addEventListener('cancel', onCancel); // Escape key
        dialog.showModal();
    });
}

function showAlertDialog(message) {
    return new Promise(function (resolve) {
        var dialog = document.getElementById('alert-dialog');
        document.getElementById('alert-dialog-message').textContent = message;
        var okBtn = document.getElementById('alert-dialog-ok');

        function cleanup() {
            okBtn.removeEventListener('click', onOk);
            dialog.removeEventListener('cancel', onOk);
            dialog.close();
            resolve();
        }
        function onOk() { cleanup(); }

        okBtn.addEventListener('click', onOk);
        dialog.addEventListener('cancel', onOk); // Escape key
        dialog.showModal();
    });
}

document.addEventListener('DOMContentLoaded', function () {
    var csrfMeta = document.querySelector('meta[name="csrf-token"]');
    var csrfToken = csrfMeta ? csrfMeta.content : '';

    document.querySelectorAll('[data-delete-url]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            showConfirmDialog(btn.dataset.confirmMessage || 'Are you sure?').then(function (confirmed) {
                if (!confirmed) {
                    return;
                }
                fetch(btn.dataset.deleteUrl, {
                    method: 'DELETE',
                    headers: { 'X-CSRF-Token': csrfToken },
                }).then(function (response) {
                    if (response.ok) {
                        window.location.href = btn.dataset.redirectUrl || window.location.href;
                        return;
                    }
                    return response.text().then(function (body) {
                        if (response.status === 409) {
                            // The server includes specifics (e.g. which teams
                            // are still using this) in the body when it has
                            // them; fall back to the button's static hint.
                            return showAlertDialog(body || btn.dataset.conflictMessage || 'This can\'t be deleted while other records still depend on it.');
                        }
                        return showAlertDialog('Something went wrong deleting this. Please try again.');
                    });
                }).catch(function (error) {
                    console.error('delete error:', error);
                    showAlertDialog('Something went wrong deleting this. Please try again.');
                });
            });
        });
    });

    document.querySelectorAll('[data-toggle-url]').forEach(function (checkbox) {
        checkbox.checked = checkbox.value === 'true';
        checkbox.addEventListener('change', function () {
            fetch(checkbox.dataset.toggleUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': csrfToken,
                },
                // UserPost.ID uses a `json:"userID,string"` tag on the Go side,
                // which requires a quoted string value, not a bare number.
                body: JSON.stringify({ userID: checkbox.dataset.userId }),
            }).catch(function (error) {
                console.error('toggle error:', error);
            });
        });
    });

    var navToggle = document.getElementById('nav-toggle');
    if (navToggle) {
        navToggle.addEventListener('click', function () {
            var menu = document.getElementById('mobile-menu');
            menu.classList.toggle('hidden');
            menu.classList.toggle('flex');
        });
    }
});
