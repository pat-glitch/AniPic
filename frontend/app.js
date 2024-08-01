document.getElementById('search-btn').addEventListener('click', function(){
    fetch('/search')
        .then(response => response.json())
        .then(data => {
            const photosDiv = document.getElementById('photos');
            photosDiv.innerHTML = '';
            data.forEach(item =>{
                const img = document.createElement('img');
                img.src = item.baseUrl;
                photosDiv.appendChild(img);
            });
        });
});