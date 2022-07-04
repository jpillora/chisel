# merge from master
git clone https://github.com/lmvlmv/chisel.git
git checkout -b jpillora-master master
git pull https://github.com/jpillora/chisel.git master
git checkout master
git merge --no-ff jpillora-master
git push origin master
