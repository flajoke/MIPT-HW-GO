package domain

type BulkError struct {
	Index int
	Error string
}
type BulkResult struct {
	Accepted int
	Rejected int
	Errors   []BulkError
	// Items not completed due to cancellation; not counted as validation rejects.
	Unprocessed int
}
