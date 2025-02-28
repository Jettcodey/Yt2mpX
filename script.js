/*
##########################################
#                 Yt2mpX             	   #
#           Made by Jettcodey            #
#                © 2025                  #
#           DO NOT REMOVE THIS           #
##########################################
*/

let downloadID = null;
let checkProgressInterval = null;

function toggleQualityDropdown() {
  const format = document.getElementById("formatSelect").value;
  const qualitySelect = document.getElementById("qualitySelect");
  const qualityLabel = document.getElementById("qualityLabel");

  if (format === "mp4") {
    qualitySelect.style.display = "inline-block";
    qualityLabel.style.display = "inline-block";
  } else {
    qualitySelect.style.display = "none";
    qualityLabel.style.display = "none";
  }
}

function startDownload() {
  const url = document.getElementById("urlInput").value.trim();
  const format = document.getElementById("formatSelect").value;
  const qualitySelect = document.getElementById("qualitySelect");

  if (!url) {
    alert("Please enter a valid YouTube URL.");
    return;
  }

  document.getElementById("statusText").innerText = "Starting download...";
  document.getElementById("progressBar").style.width = "0%";
  document.getElementById("downloadLink").style.display = "none";

  let requestBody = { url: url, format: format };

  if (format === "mp4") {
    requestBody.quality = qualitySelect.value;
  }

  fetch("http://localhost:5000/download", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(requestBody)
  })
  .then(response => response.json())
  .then(data => {
    if (data.download_id) {
      downloadID = data.download_id;
      checkProgressInterval = setInterval(checkProgress, 2000);
    } else {
      document.getElementById("statusText").innerText = "Error: Could not start download.";
    }
  })
  .catch(error => {
    console.error("Error:", error);
    document.getElementById("statusText").innerText = "Error: Unable to start download.";
  });
}

function checkProgress() {
  if (!downloadID) return;

  fetch(`http://localhost:5000/progress/${downloadID}`)
    .then(response => response.json())
    .then(data => {
      if (data.error) {
        document.getElementById("statusText").innerText = "Error: " + data.error;
        clearInterval(checkProgressInterval);
        return;
      }

      document.getElementById("progressBar").style.width = data.percentage;
      document.getElementById("statusText").innerText = "Downloading... " + data.percentage;

      if (data.completed) {
        clearInterval(checkProgressInterval);
        document.getElementById("statusText").innerText = "Download complete!";
        showDownloadLink(data.filename);
      }
    })
    .catch(error => {
      console.error("Error checking progress:", error);
      clearInterval(checkProgressInterval);
      document.getElementById("statusText").innerText = "Error fetching progress.";
    });
}

function showDownloadLink(filename) {
  const linkContainer = document.getElementById("downloadLink");
  const downloadAnchor = document.getElementById("downloadAnchor");
  downloadAnchor.href = `http://localhost:5000/get_file/${filename}`;
  linkContainer.style.display = "block";
}
