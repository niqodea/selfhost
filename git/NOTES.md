Remember to run `git update-server-info` to create the required `info/refs` file in each server repository ([ref](https://stackoverflow.com/q/12406963/8245426)).

You will also have to enable `git push` by setting `http.receivepack` to `true` in the repo config ([ref](https://adam.younglogic.com/2020/08/enabling-write-access-to-git-http-backend/)).
