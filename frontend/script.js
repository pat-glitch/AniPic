document.getElementById('loginButton').onclick = function() {
    window.location.href = '/login';
};

document.getElementById('fileInput').onchange = function(event) {
    const files = event.target.files;
    const preview = document.getElementById('imagePreview');
    preview.innerHTML = '';
    for (let file of files) {
        const img = document.createElement('img');
        img.src = URL.createObjectURL(file);
        img.height = 100;
        preview.appendChild(img);
    }
    document.getElementById('uploadButton').disabled = false;
};

document.getElementById('uploadButton').onclick = function() {
    const input = document.getElementById('fileInput');
    const files = input.files;
    const formData = new FormData();
    for (let file of files) {
        formData.append('images', file);
    }

    fetch('/upload', {
        method: 'POST',
        body: formData
    })
    .then(response => response.json())
    .then(data => {
        window.uploadedImages = data.imageUrls;
        document.getElementById('animateButton').disabled = false;
    });
};

document.getElementById('animateButton').onclick = function() {
    fetch('/animate', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ imageUrls: window.uploadedImages })
    })
    .then(response => response.json())
    .then(data => {
        const resultDiv = document.getElementById('animationResult');
        resultDiv.innerHTML = `<a href="${data.downloadUrl}">Download Animation</a>`;
    });
};
