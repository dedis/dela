package node

import (
	"fmt"
	"os"

	"go.dedis.ch/dela/cli"
)

func ExampleCLIBuilder_Build() {
	builder := NewBuilder(exampleController{})

	cmd := builder.SetCommand("bye")

	cmd.SetFlags(cli.StringFlag{
		Name:  "name",
		Usage: "set the name",
		Value: "Bob",
	})

	// This action is only executed on the CLI process. It is also possible to
	// call commands on the daemon after it has been started with "start".
	cmd.SetAction(func(flags cli.Flags) error {
		fmt.Printf("Bye, %s!", flags.String("name"))
		return nil
	})

	app := builder.Build()

	err := app.Run([]string{os.Args[0], "bye", "--name", "Alice"})
	if err != nil {
		panic("app failed: " + err.Error())
	}

	// Output: Bye, Alice!
}

// Hello is an example of a component that can be injected and resolved on the
// daemon side.
type Hello interface {
	SayTo(name string)
}

type simpleHello struct{}

func (simpleHello) SayTo(name string) {
	fmt.Printf("Hello, %s!", name)
}

// helloAction is an example of an action template to be executed on the daemon.
//
// - implements node.ActionTemplate
type helloAction struct{}

// Execute implements node.ActionTemplate. It resolves the hello component and
// say hello to the name defined by the flag.
func (tmpl helloAction) Execute(ctx Context) error {
	var hello Hello
	err := ctx.Injector.Resolve(&hello)
	if err != nil {
		return err
	}

	hello.SayTo(ctx.Flags.String("name"))

	return nil
}

// exampleController is an example of a controller passed to the builder. It
// defines the command available and the component that are injected when the
// daemon is started.
//
// - implements node.Initializer
type exampleController struct{}

// SetCommands implements node.Initializer. It defines the hello command.
func (exampleController) SetCommands(builder Builder) {
	cmd := builder.SetCommand("hello")

	// Set an action that will be executed on the daemon.
	cmd.SetAction(builder.MakeAction(helloAction{}))

	cmd.SetDescription("Say hello")
	cmd.SetFlags(cli.StringFlag{
		Name:  "name",
		Usage: "set the name",
		Value: "Bob",
	})
}

// OnStart implements node.Initializer. It injects the hello component.
func (exampleController) OnStart(flags cli.Flags, inj Injector) error {
	inj.Inject(simpleHello{})

	return nil
}

// OnStop implements node.Initializer.
func (exampleController) OnStop(Injector) error {
	return nil
}
