# Elephant Talk

Trying to figure out how Bret Victor's [Dynamicland](https://dynamicland.org/) works by trying to recreate some of it.

## What it is and what it is not

ElephantTalk is an approximation of Realtalk, a language, and RealtalkOS, an operating system, both of which make up a large part of Dynamicland. A good description of what it is like to work in such a system can be found [here](https://omar.website/posts/notes-from-dynamicland-geokit/) and in youtube videos/twitter threads here and there on the internet. The best way to experience it is to go to the physical research lab in Oakland, which I have not done. Hence I am trying to describe an elephant like so many blind men that have only touched or seen parts of it.

This project notably misses most of the collaborative environment such a system is built for. ElephantTalk is built to run on one machine, with one projector and one camera. In the future, it will support live editing of pages, but right now the main way of interacting is still through a little rectangular screen in order to set it up. Multiple projectors/screens/tables and interactions between RealtalkOS instances is out of scope.

ElephantTalk supports scripting pieces of paper (pages) through [Lisp](https://github.com/deosjr/whistle) (using an experimental interpreter I wrote in Golang) whereas Realtalk uses Lua. Image detection is done using [gocv](https://github.com/hybridgroup/gocv). The lisp package includes a very simple datalog instance, which powers the claim/wish/when model of Realtalk. There are no other dependencies: everything together is a single Go program that can run on a laptop.

I won't claim to fully understand Realtalk or dynamicland: this project's whole purpose is to tangle with their concepts and play around with them. If you find any major differences or have been to Dynamicland, please reach out and let me know! Information is still scarce, and Oakland is far away from where I live.

## How to run
The `make run` command starts the project and should open two windows. One shows camera output: this is the debug window. The other shows a mostly black screen: this is the projector window. The projector window should be moved to a second screen projected onto a wall or floor (surface). The camera should be placed such that it mostly captures the surface and is rectilinear to it. Before we can play with pages, we need to calibrate the system. For this, you will want to print out a calibration page which contains four coloured dots.

### Calibration
You should now have the projector showing a red cross on the surface. Place the calibration page so that the cross is in the center of the four dots, and with the debug window in focus press any key. This will be the center of the projection space. The more this is off from the center of the camera (which you can see on the debug window), the more we need to correct for it: this is why we are calibrating.
Now you should see another prompt to place the page, but off to the right of where it was previously. Place the calibration page so that again the cross is in the middle of the dots, and press any key. If everything is well and good, you should see a blue outline projected on top of the calibration page. Press any key one more time: this concludes calibration.

### Scripting

From now on, each frame the program will attempt to detect pages identified by coloured dots. Each page is unique and associated with a script, which runs each frame the page is detected. A database of pages is hardcoded in `main.go`. Adding pages dynamically is next on the todo list.
Currently no state is persisted between frames! Interacting with Realtalk is hard to describe in text, I suggest watching a few videos like this one https://www.youtube.com/watch?v=PvHddfHX9hc

For a good first idea of what the scripting language tries to provide beyond basic lisp, see [this image](https://omar.website/posts/notes-from-dynamicland-geokit/realtalk-cheat-sheet.png) (via [Omar Rizwan](https://twitter.com/rsnous), credited to [Tabitha Yong](https://twitter.com/telogram)).
