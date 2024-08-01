document.getElementById('loginButton').onclick = function() {
    window.location.href = '/login';
};

document.getElementById('fileInput').onchange = function(event) {
    const files = event.target.files;
    const preview = document.getElementById('imagePreview');
    preview.innerHTML = '';

    for (let i = 0; i < files.length; i++) {
        const img = document.createElement('img');
        img.src = URL.createObjectURL(files[i]);
        preview.appendChild(img);
    }

    document.getElementById('uploadButton').disabled = false;
};

document.getElementById('uploadButton').onclick = async function() {
    const files = document.getElementById('fileInput').files;
    if (files.length === 0) {
        alert('Please select images to upload.');
        return;
    }

    const formData = new FormData();
    for (let i = 0; i < files.length; i++) {
        formData.append('images', files[i]);
    }

    try {
        const response = await fetch('/upload', {
            method: 'POST',
            body: formData
        });
        const result = await response.json();
        if (response.ok) {
            document.getElementById('result').innerText = 'Images uploaded successfully!';
            document.getElementById('animateButton').disabled = false;
            sessionStorage.setItem('imageUrls', JSON.stringify(result.imageUrls));
        } else {
            document.getElementById('result').innerText = 'Failed to upload images: ' + result.error;
        }
    } catch (error) {
        document.getElementById('result').innerText = 'Failed to upload images: ' + error.message;
    }
};

document.getElementById('animateButton').onclick = async function() {
    const imageUrls = JSON.parse(sessionStorage.getItem('imageUrls'));
    if (!imageUrls || imageUrls.length === 0) {
        alert('No images to animate.');
        return;
    }

    try {
        const response = await fetch('/animate', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ imageUrls })
        });
        const result = await response.json();
        if (response.ok) {
            document.getElementById('result').innerHTML = `Animation created! <a href="${result.animationUrl}" target="_blank">View Animation</a>`;
        } else {
            document.getElementById('result').innerText = 'Failed to create animation: ' + result.error;
        }
    } catch (error) {
        document.getElementById('result').innerText = 'Failed to create animation: ' + error.message;
    }
};
