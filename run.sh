echo "Running tests"
go build
go test
echo "Starting server on 8080"
./pubsub