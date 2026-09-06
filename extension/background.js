chrome.commands.onCommand.addListener(async command=>{
  if(command==='open-bookmarkhub'){
    try{await chrome.action.openPopup()}catch{chrome.tabs.create({url:'http://127.0.0.1:17836/'})}
  }
});
