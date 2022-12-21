var helpData = [
    {
        title: "Don't Zoom!",
        content: "Zooming can mess up the editor; make sure your browser zoom is at 100%.",
        important: true
    }, {
        title: "Perfomance Concerns",
        content: "The app slows down when it starts getting thousands of keyframes; stay zoomed in during playback to get most accurate speeds.",
        important: true
    }, {
        title: "Loading/Saving",
        content: "You can change the song by clicking on the gear icon. 'Project Files' are just text files with the keyframe data, you can load/save those with the buttons."
    }, {
        title: "Navigation",
        content: "Scroll the mouse wheel to zoom, click & drag the wheel to pan, and click on the timeline to move the time."
    }, {
        title: "Selecting Keyframes",
        content: "Use LMB to select; drag to box-select; hold shift to preserve selection."
    }, {
        title: "Adding/Deleting Keyframes",
        content: "Use RMB to add a new (off) keyframe. Hold Shift to make it an on keyframe, and/or hold Control to add a keyframe to each channel at the same time."
    }, {
        title: "Manipulating Keyframes",
        content: "Press G grab & drag keyframes, then LMB to place or RMB to cancel. Use I to invert values, use A to align accross channels"
    }, {
		title: "Copy & Paste",
		content: "Press D to duplicate selected keyframes"
	}, {
		title: "Duplicate Removal",
		content: "Press R to remove duplicate overlapping keyframes (recommended before export)"
	}
];

function createHelpCard(t, c, i) {

	let color = (i == true) ? "red darken-2" : "blue-grey darken-2";
	
	console.log(color);
	
    let newCard = $("<div/>", {
        class: "card " + color + " darken-1"
    });

    let newCardContent = $("<div/>", {
        class: "card-content white-text"
    });

    let newCardTitle = $("<span/>", {
        class: "card-title"
    }).html(t);

    let newCardParagraph = $("<p/>").html(c);

    newCardContent.append(newCardTitle);
    newCardContent.append(newCardParagraph);
    newCard.append(newCardContent);

    return newCard;
}

for (let i = 0; i < helpData.length; i++) {

    let title = helpData[i].title;
    let content = helpData[i].content;
    let important = helpData[i].important;

    let newCard = createHelpCard(title, content, important);

    $("#help-container").append(newCard);
}