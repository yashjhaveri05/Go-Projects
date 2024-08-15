// Added Comments Using ChatGPT
// Refered Youtube Video

package main

import(
	"fmt"                // For formatted I/O operations (e.g., printing to the console)
	"os"                 // For file operations (e.g., checking if files exist, creating directories)
	"encoding/json"      // For JSON operations (e.g., encoding and decoding JSON)
	"io/ioutil"          // For reading from and writing to files
	"path/filepath"      // For file path operations (e.g., joining directory and file names)
	"sync"               // For synchronization primitives (e.g., mutexes to handle concurrent access)
	"github.com/jcelliott/lumber"  // A third-party logging library for structured logging
)

// Interface defining the methods for logging at different levels of severity
type Logger interface{
	Fatal(string, ...interface{})   // Logs fatal errors that may stop the program
	Error(string, ...interface{})   // Logs non-fatal errors
	Warn(string, ...interface{})    // Logs warnings
	Info(string, ...interface{})    // Logs general informational messages
	Debug(string, ...interface{})   // Logs debug-level information for developers
	Trace(string, ...interface{})   // Logs detailed trace information
}

// Struct representing the database driver that handles the storage and retrieval of data
type Driver struct{
	mutex sync.Mutex               // Mutex to protect access to the `mutexes` map
	mutexes map[string]*sync.Mutex // Map of collection names to mutexes, used to handle concurrent access to collections
	dir string                     // Base directory where all collections are stored
	log Logger                     // Logger instance for logging messages
}

// Struct representing options for configuring the database driver
type Options struct{
	Logger  // Embeds the Logger interface to allow custom logging
}

// Function to create a new database driver instance
// It initializes the base directory and logging options, and ensures that the directory exists
func New(dir string, options *Options) (*Driver, error){
	// Clean up the directory path by removing any redundant elements
	dir = filepath.Clean(dir)
	
	// Initialize default options, or use the provided ones
	opts := Options{}
	if options != nil {
		opts = *options
	}
	
	// If no custom logger is provided, use a default console logger with INFO level
	if opts.Logger == nil {
		opts.Logger = lumber.NewConsoleLogger(lumber.INFO)
	}
	
	// Create a new Driver instance with the given directory and logger
	driver := Driver{
		dir: dir,
		mutexes: make(map[string]*sync.Mutex),  // Initialize the map for mutexes
		log: opts.Logger,
	}

	// Check if the directory already exists
	if _, err := os.Stat(dir); err == nil {
		opts.Logger.Debug("Using '%s' (database already exists)\n", dir)
		return &driver, nil
	}
	
	// If the directory does not exist, create it and log the action
	opts.Logger.Debug("Creating database at '%s'\n", dir)
	return &driver, os.MkdirAll(dir, 0755)  // Create the directory with appropriate permissions
}

// Method to insert a record into the database
// It saves the data as a JSON file in the specified collection and resource name
func (d *Driver) Insert(collection, resource string, v interface{}) error {
	// Validate that a collection name is provided
	if collection == "" {
		return fmt.Errorf("Missing Collection - no place to save record")
	}
	
	// Validate that a resource name is provided
	if resource == "" {
		return fmt.Errorf("Missing Resource - unable to save record (no name)")
	}
	
	// Obtain or create a mutex for the collection to ensure thread-safe access
	mutex := d.getOrCreateMutex(collection)
	mutex.Lock()              // Lock the mutex to prevent concurrent writes
	defer mutex.Unlock()      // Ensure the mutex is unlocked after the function finishes

	// Construct the directory path for the collection and the final file path for the resource
	dir := filepath.Join(d.dir, collection)
	finalPath := filepath.Join(dir, resource + ".json")
	tempPath := finalPath + ".tmp"  // Use a temporary file path to ensure safe file writing

	// Ensure the collection directory exists, creating it if necessary
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Convert the data (v) to a pretty-printed JSON format
	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return err
	}

	// Append a newline character to the JSON data for readability
	b = append(b, byte('\n'))
	
	// Write the JSON data to a temporary file
	if err := ioutil.WriteFile(tempPath, b, 0644); err != nil {
		return err
	}

	// Rename the temporary file to the final file path, making the write operation atomic
	return os.Rename(tempPath, finalPath)
}

// Method to read a single record from the database
// It reads the JSON file for the specified collection and resource, and unmarshals it into the provided struct
func (d *Driver) Read(collection, resource string, v interface{}) error {
	// Validate that a collection name is provided
	if collection == "" {
		return fmt.Errorf("Missing Collection - unable to read records")
	}
	
	// Validate that a resource name is provided
	if resource == "" {
		return fmt.Errorf("Missing Resource - unable to read record (no name)")
	}
	
	// Construct the file path for the resource's JSON file
	record := filepath.Join(d.dir, collection, resource + ".json")

	// Check if the file exists
	if _, err := stat(record); err != nil {
		return err
	}

	// Read the JSON data from the file
	b, err := ioutil.ReadFile(record)
	if err != nil {
		return err
	}

	// Unmarshal the JSON data into the provided struct (v)
	return json.Unmarshal(b, &v)
}

// Method to read all records from a collection
// It reads all JSON files in the collection directory and returns their contents as a slice of strings
func (d *Driver) ReadAll(collection string) ([]string, error){
	// Validate that a collection name is provided
	if collection == "" {
		return nil, fmt.Errorf("Missing Collection - unable to read records")
	}
	
	// Construct the directory path for the collection
	dir := filepath.Join(d.dir, collection)

	// Check if the directory exists
	if _, err := stat(dir); err != nil {
		return nil, err
	}

	// Read the list of files in the collection directory
	files, _ := ioutil.ReadDir(dir)

	// Initialize a slice to hold the contents of all records
	var records []string
	for _, file := range files {
		if file.IsDir() {
			continue  // Skip directories, as we are only interested in files
		}
		
		// Read the contents of each file and append it to the records slice
		b, err := ioutil.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return nil, err
		}
		records = append(records, string(b))
	}
	return records, nil
}

// Method to delete a record from the database
// It deletes the specified file or directory from the collection
func (d *Driver) Delete(collection, resource string) error {
	// Construct the path for the resource within the collection
	path := filepath.Join(collection, resource)
	
	// Obtain or create a mutex for the collection to ensure thread-safe access
	mutex := d.getOrCreateMutex(collection)
	mutex.Lock()              // Lock the mutex to prevent concurrent deletions
	defer mutex.Unlock()      // Ensure the mutex is unlocked after the function finishes
	
	// Construct the full path for the resource
	dir := filepath.Join(d.dir, path)
	
	// Determine whether the resource is a file or directory, and delete it accordingly
	switch fi, err := stat(dir); {
		case fi == nil, err != nil:  // If the file or directory does not exist, return an error
			return fmt.Errorf("unable to find file or directory named %v \n", path)
		case fi.Mode().IsDir():      // If the path is a directory, delete the entire directory
			return os.RemoveAll(dir)
		case fi.Mode().IsRegular():  // If the path is a regular file, delete the file with the ".json" extension
			return os.RemoveAll(dir + ".json")
	}
	return nil
}

// Helper function to get or create a mutex for a given collection
// Ensures that each collection has its own mutex to handle concurrent access
func (d *Driver) getOrCreateMutex(collection string) *sync.Mutex {
	d.mutex.Lock()              // Lock the main mutex to protect the `mutexes` map
	defer d.mutex.Unlock()      // Ensure the main mutex is unlocked after the function finishes
	
	// Check if a mutex already exists for the collection
	m, ok := d.mutexes[collection]
	if !ok {
		// If not, create a new mutex and store it in the map
		m = &sync.Mutex{}
		d.mutexes[collection] = m
	}
	return m
}

// Helper function to check if a file exists with the given path
// Also checks for the existence of a file with a ".json" extension if the original path does not exist
func stat(path string) (fi os.FileInfo, err error) {
	if fi, err = os.Stat(path); os.IsNotExist(err) {
		fi, err = os.Stat(path + ".json")  // Check if a ".json" file exists with the same name
	}
	return
}

// Struct to represent an address with various fields
type Address struct{
	City string           // City of the address
	State string          // State of the address
	Country string        // Country of the address
	Pincode json.Number   // Pincode, stored as a JSON number to preserve precision
}

// Struct to represent a user with various fields
type User struct{
	Name string           // Name of the user
	Age json.Number       // Age, stored as a JSON number to preserve precision
	Contact string        // Contact number of the user
	Company string        // Company the user works for
	Address Address       // Address of the user, represented as an Address struct
}

// Main function to demonstrate the usage of the database driver
func main(){
	dir := "./"  // Path to store the database with the individual collections

	// Create a new database driver with the specified directory
	db, err := New(dir, nil)
	if err != nil {
		fmt.Println("Error", err)
	}

	// Static database records representing multiple users
	employees := []User{
		{"John Doe", "30", "1234567890", "Google", Address{"Bangalore", "Karnataka", "India", "560001"}},
		{"Jane Doe", "25", "0987654321", "Microsoft", Address{"Hyderabad", "Telangana", "India", "500001"}},
		{"John Smith", "35", "1234509876", "Apple", Address{"Chennai", "Tamil Nadu", "India", "600001"}},
		{"Jane Smith", "28", "0987612345", "Amazon", Address{"Mumbai", "Maharashtra", "India", "400001"}},
		{"Tom Doe", "30", "1234567891", "Google", Address{"Bangalore", "Karnataka", "India", "560002"}},
		{"Tim Doe", "25", "0987654322", "Microsoft", Address{"Hyderabad", "Telangana", "India", "500007"}},
		{"Tom Smith", "35", "1234509873", "Apple", Address{"Chennai", "Tamil Nadu", "India", "600005"}},
		{"Tim Smith", "28", "0987612344", "Amazon", Address{"Mumbai", "Maharashtra", "India", "400008"}},
	}

	// Insert each user record into the "users" collection in the database
	for _, value := range employees {
		db.Insert("users", value.Name, User{
			Name: value.Name,
			Age: value.Age,
			Contact: value.Contact,
			Company: value.Company,
			Address: value.Address,
		})
	}

	// Read all records from the "users" collection
	records, err := db.ReadAll("users")
	if err != nil {
		fmt.Println("Error", err)
	}
	
	// Print the raw JSON records (still in string format)
	fmt.Println(records)

	// Unmarshal each JSON string into a User struct and store them in the allusers slice
	allusers := []User{}
	for _, f := range records {
		employeeFound := User{}
		if err := json.Unmarshal([]byte(f), &employeeFound); err != nil {
			fmt.Println("Error", err)
		}
		allusers = append(allusers, employeeFound)
	}

	// Print the slice of User structs to show the parsed data
	fmt.Println(allusers)

	// Uncomment the following code to demonstrate deleting records from the database

	// Delete a specific user record from the "users" collection
	// if err := db.Delete("users", "John Doe"); err != nil {
	// 	fmt.Println("Error", err)
	// }

	// Attempt to delete all records from the "users" collection
	// if err := db.Delete("users", ""); err != nil {
	// 	fmt.Println("Error", err)
	// }
}