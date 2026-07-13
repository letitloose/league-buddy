document.addEventListener('DOMContentLoaded', function () {
    var csrfMeta = document.querySelector('meta[name="csrf-token"]');
    var csrfToken = csrfMeta ? csrfMeta.content : '';

    document.querySelectorAll('[data-delete-url]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            if (!confirm(btn.dataset.confirmMessage || 'Are you sure?')) {
                return;
            }
            fetch(btn.dataset.deleteUrl, {
                method: 'DELETE',
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (response) {
                if (response.ok) {
                    window.location.href = btn.dataset.redirectUrl || window.location.href;
                }
            }).catch(function (error) {
                console.error('delete error:', error);
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
